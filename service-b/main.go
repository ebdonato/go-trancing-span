package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/ebdonato/go-deploy-cloud-run/pkg/cep"
	"github.com/ebdonato/go-deploy-cloud-run/pkg/weather"
	"github.com/ebdonato/go-deploy-cloud-run/util"
	"github.com/go-chi/chi/v5"
)

func main() {
	port := util.GetEnvVariable("PORT_SB")
	apiKey := util.GetEnvVariable("WEATHER_API_KEY")

	r := chi.NewRouter()
	r.Get("/{cep}", handlerCEP(apiKey))

	log.Println("Starting web server B on port", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), r))
}

type serviceResponse struct {
	Location    string
	Temperature weather.Temperature
}

func handlerCEP(apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cepParams := strings.TrimSpace(r.URL.Path[1:])

		viaCep := cep.InstanceViaCep()
		location, err := viaCep.FindLocation(cepParams)
		if err != nil {
			message := "Invalid CEP"
			log.Println(message)
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(message))
			return
		}

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

		response := serviceResponse{
			Location:    location,
			Temperature: temperature,
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}
