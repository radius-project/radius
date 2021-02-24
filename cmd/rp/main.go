// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/Azure/radius/pkg/curp"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/curp/db"
	"github.com/Azure/radius/pkg/curp/k8sauth"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// App Service uses this env-var to tell us what port to listen on.
	port, ok := os.LookupEnv("PORT")
	if !ok {
		log.Fatalln("env: PORT is required")
	}

	connString, ok := os.LookupEnv("MONGODB_CONNECTION_STRING")
	if !ok {
		log.Fatalln("env: MONGODB_CONNECTION_STRING is required")
	}

	dbName, ok := os.LookupEnv("MONGODB_DATABASE")
	if !ok {
		log.Fatalln("env: MONGODB_DATABASE is required")
	}

	authenticate := true
	skipAuth, ok := os.LookupEnv("SKIP_AUTH")
	if ok && skipAuth == "true" {
		log.Println("Authentication will be skipped! This is a development-time only setting")
		authenticate = false
	}

	k8s, err := k8sauth.CreateClient()
	if err != nil {
		log.Printf("error connecting to kubernetes: %s", err)
		panic(err)
	}

	arm, err := armauth.GetArmConfig()
	if err != nil {
		log.Printf("error connecting to ARM: %s", err)
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// cosmos does not support retrywrites, but it's the default for the golang driver
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(connString).SetRetryWrites(false))
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		_ = client.Disconnect(ctx)
	}()

	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	db := db.NewCurpDB(client.Database(dbName))

	addr := ":" + port
	log.Printf("listening on: '%s'...", addr)
	server := curp.NewServer(db, arm, k8s, addr, curp.ServerOptions{Authenticate: authenticate})
	err = server.ListenAndServe()
	if err != nil {
		panic(err)
	}
	log.Println("shutting down...")
}
