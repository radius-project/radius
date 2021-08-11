// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Azure/radius/pkg/model"
	azmodel "github.com/Azure/radius/pkg/model/azure"
	k8smodel "github.com/Azure/radius/pkg/model/kubernetes"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp"
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/deployment"
	"github.com/Azure/radius/pkg/radrp/k8sauth"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	if ok && strings.EqualFold(skipAuth, "true") {
		log.Println("Authentication will be skipped! This is a development-time only setting")
		authenticate = false
	}

	var k8s *client.Client
	var err error
	skipKubernetes, ok := os.LookupEnv("SKIP_K8S")
	if ok && strings.EqualFold(skipKubernetes, "true") {
		log.Println("skipping Kubernetes connection...")
	} else {
		k8s, err = k8sauth.CreateClient()
		if err != nil {
			log.Printf("error connecting to kubernetes: %s", err)
			panic(err)
		}
	}

	var arm armauth.ArmConfig
	skipARM, ok := os.LookupEnv("SKIP_ARM")
	if ok && strings.EqualFold(skipARM, "true") {
		arm = armauth.ArmConfig{}
	} else {
		arm, err = armauth.GetArmConfig()
		if err != nil {
			log.Printf("error connecting to ARM: %s", err)
			panic(err)
		}
	}

	var appmodel model.ApplicationModel
	if os.Getenv("RADIUS_MODEL") == "" || strings.EqualFold(os.Getenv("RADIUS_MODEL"), "azure") {
		appmodel = azmodel.NewAzureModel(arm, k8s)
	} else if strings.EqualFold(os.Getenv("RADIUS_MODEL"), "k8s") {
		appmodel = k8smodel.NewKubernetesModel(k8s)
	} else {
		log.Fatal(fmt.Errorf("unknown value for RADIUS_MODEL '%s'", os.Getenv("RADIUS_MODEL")))
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

	logger, flushLogs, err := radlogger.NewLogger(fmt.Sprintf("radRP-%s", arm.ResourceGroup))
	if err != nil {
		panic(err)
	}
	defer flushLogs()
	logger = logger.WithValues(
		radlogger.LogFieldResourceGroup, arm.ResourceGroup,
		radlogger.LogFieldSubscriptionID, arm.SubscriptionID)

	options := radrp.ServerOptions{
		Address:      ":" + port,
		Authenticate: authenticate,
		Deploy:       deployment.NewDeploymentProcessor(appmodel),
		DB:           db.NewRadrpDB(client.Database(dbName)),
		Logger:       logger,
	}

	logger.Info(fmt.Sprintf("listening on: '%s'...", options.Address))
	server := radrp.NewServer(options)
	err = server.ListenAndServe()
	if err != nil {
		panic(err)
	}

	logger.Info("shutting down...")
}
