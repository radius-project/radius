package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
		log.Println("Failed to start magpie server")
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
	log.Println("Get the status")
	if req.Method != "GET" {
		log.Print("Method not supported")
		writeResponseHeader(res, http.StatusMethodNotAllowed, nil)
		res.Header().Set("Allow", "GET")
		return
	}
	var b []byte
	var err error
	bdings := bindings.LoadBindings(Providers)
	healthy := false
	if bdings != nil {
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
		b, err = json.Marshal(bindingStatuses)
		if err != nil {
			log.Println("error marshaling status to json - ", err)
			writeResponseHeader(res, 500, errors.New("Error getting status"))
			return
		}
	}
	log.Println(fmt.Sprintf("The binding statuses are %s and the status is %t", string(b), healthy))
	if healthy {
		writeResponseHeader(res, 200, nil)
		res.Header().Set("Content-Type", "application/json")
		res.Write(b)
	} else {
		writeResponseHeader(res, 500, errors.New("Error getting status"))
	}
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
			log.Println("Error response failed on writing ", size, " bytes with error ", err)
		}
	}
}
