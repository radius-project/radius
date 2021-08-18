// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radrp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/model/azure"
	"github.com/Azure/radius/pkg/model/kubernetes"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/deployment"
	"github.com/Azure/radius/pkg/radrp/k8sauth"
	"github.com/go-logr/logr"
	"go.mongodb.org/mongo-driver/mongo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StartRadRP creates and starts the Radius RP
func StartRadRP(ctx context.Context, arm armauth.ArmConfig, dbClient *mongo.Client, dbName string, healthChannels healthcontract.HealthChannels) {
	// App Service uses this env-var to tell us what port to listen on.
	port, ok := os.LookupEnv("PORT")
	if !ok {
		log.Fatalln("env: PORT is required")
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

	authenticate := true
	skipAuth, ok := os.LookupEnv("SKIP_AUTH")
	if ok && strings.EqualFold(skipAuth, "true") {
		log.Println("Authentication will be skipped! This is a development-time only setting")
		authenticate = false
	}

	logger, flushLogs, err := radlogger.NewLogger(fmt.Sprintf("radRP-%s-%s", arm.SubscriptionID, arm.ResourceGroup))
	if err != nil {
		panic(err)
	}
	defer flushLogs()
	logger = logger.WithValues(
		radlogger.LogFieldResourceGroup, arm.ResourceGroup,
		radlogger.LogFieldSubscriptionID, arm.SubscriptionID)

	db := db.NewRadrpDB(dbClient.Database(dbName))

	var appmodel model.ApplicationModel
	if os.Getenv("RADIUS_MODEL") == "" || strings.EqualFold(os.Getenv("RADIUS_MODEL"), "azure") {
		appmodel = azure.NewAzureModel(arm, k8s)
	} else if strings.EqualFold(os.Getenv("RADIUS_MODEL"), "k8s") {
		appmodel = kubernetes.NewKubernetesModel(k8s)
	} else {
		log.Fatal(fmt.Errorf("unknown value for RADIUS_MODEL '%s'", os.Getenv("RADIUS_MODEL")))
	}

	options := ServerOptions{
		Address:      ":" + port,
		Authenticate: authenticate,
		Deploy:       deployment.NewDeploymentProcessor(appmodel, &healthChannels),
		DB:           db,
		Logger:       logger,
	}

	changeListener := NewChangeListener(db, &healthChannels)
	ctx = logr.NewContext(ctx, logger)
	go changeListener.ListenForChanges(ctx)
	server := NewServer(options)
	go exitListener(ctx, server)

	logger.Info(fmt.Sprintf("listening on: '%s'...", options.Address))
	err = server.ListenAndServe()
	if err != nil {
		panic(err)
	}

	logger.Info("RadRP stopped...")
}

func exitListener(ctx context.Context, server *http.Server) {
	logger := radlogger.GetLogger(ctx)
	const waitDuration = time.Second * 10
	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping RadRP...")
			err := server.Shutdown(ctx)
			if err != nil {
				logger.Error(err, "Error gracefully shutting down RadRP...")
			}
			break
		case <-time.After(waitDuration):
			continue
		}
	}
}
