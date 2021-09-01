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

	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/model/azure"
	"github.com/Azure/radius/pkg/model/kubernetes"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/deployment"
	"github.com/Azure/radius/pkg/service"
	"github.com/go-logr/logr"
)

// StartRadRP creates and starts the Radius RP
func StartRadRP(ctx context.Context, options service.Options) {
	// App Service uses this env-var to tell us what port to listen on.
	port, ok := os.LookupEnv("PORT")
	if !ok {
		log.Fatalln("env: PORT is required")
	}

	authenticate := true
	skipAuth, ok := os.LookupEnv("SKIP_AUTH")
	if ok && strings.EqualFold(skipAuth, "true") {
		log.Println("Authentication will be skipped! This is a development-time only setting")
		authenticate = false
	}

	logger, flushLogs, err := radlogger.NewLogger(fmt.Sprintf("radRP-%s-%s", options.Arm.SubscriptionID, options.Arm.ResourceGroup))
	if err != nil {
		panic(err)
	}
	defer flushLogs()
	logger = logger.WithValues(
		radlogger.LogFieldResourceGroup, options.Arm.ResourceGroup,
		radlogger.LogFieldSubscriptionID, options.Arm.SubscriptionID)

	db := db.NewRadrpDB(options.DBClient.Database(options.DBName))

	var appmodel model.ApplicationModel
	if os.Getenv("RADIUS_MODEL") == "" || strings.EqualFold(os.Getenv("RADIUS_MODEL"), "azure") {
		appmodel = azure.NewAzureModel(options.Arm, options.K8s)
	} else if strings.EqualFold(os.Getenv("RADIUS_MODEL"), "k8s") {
		appmodel = kubernetes.NewKubernetesModel(options.K8s)
	} else {
		log.Fatal(fmt.Errorf("unknown value for RADIUS_MODEL '%s'", os.Getenv("RADIUS_MODEL")))
	}

	serverOptions := ServerOptions{
		Address:      ":" + port,
		Authenticate: authenticate,
		Deploy:       deployment.NewDeploymentProcessor(appmodel, &options.HealthChannels),
		DB:           db,
		Logger:       logger,
	}

	changeListener := NewChangeListener(db, &options.HealthChannels)
	ctx = logr.NewContext(ctx, logger)
	go changeListener.ListenForChanges(ctx)
	server := NewServer(serverOptions)
	go exitListener(ctx, server)

	logger.Info(fmt.Sprintf("listening on: '%s'...", serverOptions.Address))
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
