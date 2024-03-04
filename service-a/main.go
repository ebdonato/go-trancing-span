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
)

type serviceResponse struct {
	Location    string
	Temperature weather.Temperature
}

func main() {
	port := util.GetEnvVariable("PORT_SA")
	serviceUrl := util.GetEnvVariable("SERVICE_URL") + "/%s"

	r := chi.NewRouter()
	r.Get("/{cep}", handlerCEP(serviceUrl))

	log.Println("Starting web server A on port", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), r))
}

func handlerCEP(serviceUrl string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cepParams := strings.TrimSpace(r.URL.Path[1:])

		if !util.IsValidCEP(cepParams) {
			message := "Invalid CEP"
			log.Println(message)
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(message))
			return
		}

		url := fmt.Sprintf(serviceUrl, cepParams)

		req, err := http.Get(url)
		if err != nil {
			w.WriteHeader(req.StatusCode)
			w.Write([]byte(req.Status))
			return
		}
		defer req.Body.Close()

		res, err := io.ReadAll(req.Body)
		if err != nil {
			message := "Internal Server Error"
			log.Println(message)
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(message))
			return
		}

		var data serviceResponse
		err = json.Unmarshal(res, &data)
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
