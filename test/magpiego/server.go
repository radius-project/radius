package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/test/magpiego/bindings"
)

var server *http.Server

const (
	backendURI = "/backend"
	healthURI  = "/healthz"
	port       = "3000"
)

var Providers = map[string]bindings.BindingProvider{
	"DAPRPUBSUB":      bindings.DaprPubSubBinding,
	"KEYVAULT":        bindings.KeyVaultBinding,
	"MONGODB":         bindings.MongoBinding,
	"SERVICEBUS":      bindings.ServiceBusBinding,
	"SQL":             bindings.MicrosoftSqlBinding,
	"REDIS":           bindings.RedisBinding,
	"DAPRSTATESTORE":  bindings.DaprStateStoreBinding,
	"RABBITMQ":        bindings.RabbitMQBinding,
	"DAPRSECRETSTORE": bindings.DaprSecretStoreBinding,
}

func startMagpieServer() error {
	mux := setupServeMux()
	server = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	err := server.ListenAndServe()
	if err != nil {
		log.Println("Error starting magpie server")
		return err
	}
	return nil
}

func setupServeMux() *mux.Router {
	router := mux.NewRouter()
	router.Handle(backendURI, http.HandlerFunc(backendHandler)).Methods("GET")
	router.Handle(healthURI, http.HandlerFunc(statusHandler)).Methods("GET")
	return router
}

func statusHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		log.Print("Method not supported")
		writeResponse(res, http.StatusMethodNotAllowed, nil)
		res.Header().Set("Allow", "GET")
		return
	}
	var b []byte
	var err error
	bdings := bindings.LoadBindings(Providers)
	healthy := true
	if bdings != nil {
		var bindingStatuses []bindings.BindingStatus
		for _, binding := range bdings {
			bindingStatus := binding.BindingProviders(binding.EnvVars)
			bindingStatuses = append(bindingStatuses, bindingStatus)
			if !bindingStatus.Ok {
				healthy = false
			}
		}
		b, err = json.Marshal(bindingStatuses)
		if err != nil {
			log.Println("error marshaling status to json - ", err)
			writeResponse(res, 500, []byte("error marshaling status to json"))
			return
		}
	}
	if healthy {
		writeResponse(res, 200, b)
	} else {
		writeResponse(res, 500, b)
	}
}

func backendHandler(res http.ResponseWriter, req *http.Request) {
	writeResponse(res, 200, []byte("backend call response"))
}

func writeResponse(res http.ResponseWriter, status int, b []byte) {
	res.WriteHeader(status)
	res.Header().Set("Content-Type", "application/json")
	size, err := res.Write(b)
	if err != nil {
		log.Println("Error writing response of size - ", size, " err - ", err.Error())
	}
}
