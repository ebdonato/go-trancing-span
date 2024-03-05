package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/ebdonato/go-deploy-cloud-run/pkg/weather"
	"github.com/ebdonato/go-deploy-cloud-run/util"
	"github.com/go-chi/chi/v5"

	"context"
	"flag"
	"os"
	"os/signal"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

var logger = log.New(os.Stderr, "zipkin-example", log.Ldate|log.Ltime|log.Llongfile)

// initTracer creates a new trace provider instance and registers it as global trace provider.
func initTracer(url string) (func(context.Context) error, error) {
	// Create Zipkin Exporter and install it as a global tracer.
	//
	// For demoing purposes, always sample. In a production application, you should
	// configure the sampler to a trace.ParentBased(trace.TraceIDRatioBased) set at the desired
	// ratio.
	exporter, err := zipkin.New(
		url,
		zipkin.WithLogger(logger),
	)
	if err != nil {
		return nil, err
	}

	batcher := sdktrace.NewBatchSpanProcessor(exporter)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(batcher),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("zipkin-test"),
		)),
	)
	otel.SetTracerProvider(tp)

	// Set global propagator for context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

type serviceResponse struct {
	Location    string
	Temperature weather.Temperature
}

func main() {
	url := flag.String("zipkin", "http://localhost:9411/api/v2/spans", "zipkin url")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	shutdown, err := initTracer(*url)
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Fatal("failed to shutdown TracerProvider: %w", err)
		}
	}()

	port := util.GetEnvVariable("PORT_SA")
	serviceUrl := util.GetEnvVariable("SERVICE_URL") + "/%s"

	r := chi.NewRouter()
	r.Get("/{cep}", handlerCEP(serviceUrl))

	log.Println("Starting web server A on port", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), r))
}

func handlerCEP(serviceUrl string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		carrier := propagation.HeaderCarrier(r.Header)
		ctx := r.Context()
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)

		tr := otel.GetTracerProvider().Tracer("handlerCEP")
		ctx, span := tr.Start(ctx, "service-a", trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		cepParams := strings.TrimSpace(r.URL.Path[1:])

		if !util.IsValidCEP(cepParams) {
			message := "Invalid CEP"
			log.Println(message)
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(message))
			return
		}

		url := fmt.Sprintf(serviceUrl, cepParams)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			message := "Internal Server Error"
			log.Println(message)
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(message))
			return
		}

		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			w.WriteHeader(res.StatusCode)
			w.Write([]byte(res.Status))
			return
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			message := "Internal Server Error"
			log.Println(message)
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(message))
			return
		}

		var data serviceResponse
		err = json.Unmarshal(body, &data)
		if err != nil {
			message := "Parse response from service failed"
			log.Println(message)
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(message))
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(data)
	}
}
