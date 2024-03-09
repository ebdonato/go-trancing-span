package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/ebdonato/go-deploy-cloud-run/pkg/cep"
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

	port := util.GetEnvVariable("PORT_SB")
	apiKey := util.GetEnvVariable("WEATHER_API_KEY")

	r := chi.NewRouter()
	r.Get("/cep/{cep}", handlerCEP())
	r.Get("/location/{location}", handlerLocation(apiKey))

	log.Println("Starting web server B on port", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), r))
}

func handlerCEP() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		carrier := propagation.HeaderCarrier(r.Header)
		ctx := r.Context()
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)

		tr := otel.GetTracerProvider().Tracer("handlerCEP")
		_, span := tr.Start(ctx, "service-b-cep", trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		cepParams := chi.URLParam(r, "cep")

		viaCep := cep.InstanceViaCep()
		location, err := viaCep.FindLocation(cepParams)
		if err != nil {
			var message string
			var statusCode int

			if err.Error() == "CEP NOT FOUND" {
				message = "CEP not found"
				statusCode = http.StatusNotFound
			} else {
				message = "Invalid CEP"
				statusCode = http.StatusUnprocessableEntity
			}

			log.Println(message)
			log.Println(err)
			w.WriteHeader(statusCode)
			w.Write([]byte(message))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(location))
	}
}

func handlerLocation(apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		carrier := propagation.HeaderCarrier(r.Header)
		ctx := r.Context()
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)

		tr := otel.GetTracerProvider().Tracer("handlerCEP")
		_, span := tr.Start(ctx, "service-b-location", trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		location := chi.URLParam(r, "location")

		weatherApi := weather.InstanceWeatherApi(apiKey)
		temperature, err := weatherApi.GetTemperature(location)

		if err != nil {
			message := "Internal Server Error"
			log.Println(message)
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(message))
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(temperature)
	}
}
