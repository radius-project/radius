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
	"DAPRPUBSUB":     bindings.DaprPubSubBinding,
	"KEYVAULT":       bindings.KeyVaultBinding,
	"MONGODB":        bindings.MongoBinding,
	"SERVICEBUS":     bindings.ServiceBusBinding,
	"SQL":            bindings.MicrosoftSqlBinding,
	"REDIS":          bindings.RedisBinding,
	"DAPRSTATESTORE": bindings.DaprStateStoreBinding,
	"RABBITMQ":       bindings.RabbitMQBinding,
}

func startMagpieServer() error {
	mux := setupServeMux()
	server = &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal("Failed to start magpie server")
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
		writeResponseHeader(res, http.StatusMethodNotAllowed, nil)
		res.Header().Set("Allow", "GET")
		return
	}
	bdings := bindings.LoadBindings(Providers)
	healthy := false
	var bindingStatuses []bindings.BindingStatus
	for _, binding := range bdings {
		bindingStatus := binding.BindingProviders(binding.EnvVars)
		bindingStatuses = append(bindingStatuses, bindingStatus)
		if !bindingStatus.Ok {
			healthy = false
		} else {
			healthy = true
		}
	}
	b, err := json.Marshal(bindingStatuses)
	if err != nil {
		log.Fatal("error marshaling status to json - ", err)
		writeResponseHeader(res, 500, nil)
		return
	}
	if healthy {
		writeResponseHeader(res, 200, nil)
	} else {
		writeResponseHeader(res, 500, nil)
	}
	res.Header().Set("Content-Type", "application/json")
	res.Write(b)
}

func backendHandler(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "application/json")
	writeResponseHeader(res, 200, nil)
	res.Write([]byte("backend call response"))
}

func writeResponseHeader(res http.ResponseWriter, status int, err error) {
	res.WriteHeader(status)
	if err != nil {
		size, err := res.Write([]byte(err.Error()))
		if err != nil {
			log.Fatal("Error response failed on writing ", size, " bytes with error ", err)
		}
	}
}
