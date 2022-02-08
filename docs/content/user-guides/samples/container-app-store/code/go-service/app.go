package main

import (
	"log"
	"net/http"

	dapr "github.com/dapr/go-sdk/client"
	"github.com/gorilla/mux"
)

type App struct {
	Router     *mux.Router
	daprClient dapr.Client
}

func (a *App) Initialize(client dapr.Client) {
	a.daprClient = client
	a.Router = mux.NewRouter()

	a.Router.HandleFunc("/", a.Hello).Methods("GET")
	a.Router.HandleFunc("/inventory", a.GetInventory).Methods("GET")
}

func (a *App) Hello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello world! It's me"))
}

func (a *App) GetInventory(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Inventory in stock"))
}

func (a *App) Run(addr string) {
	log.Fatal(http.ListenAndServe(addr, a.Router))
}
