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
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/health"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radrp"
	"github.com/Azure/radius/pkg/radrp/k8sauth"
	"github.com/Azure/radius/pkg/service"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	k8sClient "k8s.io/client-go/kubernetes"
	k8sRClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ChannelBufferSize defines the buffer size for health registration channel
const ChannelBufferSize = 100

func main() {
	// Create DB client and connect
	connString, ok := os.LookupEnv("MONGODB_CONNECTION_STRING")
	if !ok {
		log.Fatalln("env: MONGODB_CONNECTION_STRING is required")
	}

	dbName, ok := os.LookupEnv("MONGODB_DATABASE")
	if !ok {
		log.Fatalln("env: MONGODB_DATABASE is required")
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

	arm, err := getArmConfig()
	if err != nil {
		panic(fmt.Sprintf("error connecting to ARM: %s", err))
	}

	var k8s *k8sRClient.Client
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

	// Create kubernetes clientset
	cfg, err := k8sauth.GetConfig()
	if err != nil {
		log.Printf("error getting kubernetes config: %s", err)
		panic(err)
	}

	k8sClientSet, err := k8sClient.NewForConfig(cfg)
	if err != nil {
		log.Printf("error connecting to kubernetes: %s", err)
		panic(err)
	}

	// Create a channel to handle the shutdown
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancelFunc := context.WithCancel(context.Background())

	healthChannels := makeHealthChannels()

	options := service.Options{
		Arm:            arm,
		K8sClient:      k8s,
		K8sClientSet:   k8sClientSet,
		DBClient:       client,
		DBName:         dbName,
		HealthChannels: healthChannels,
	}
	go radrp.StartRadRP(ctx, options)
	go health.StartRadHealth(ctx, options)

	waitDuration := time.Second * 10
	for {
		select {
		case <-exitCh:
			fmt.Println("Shutting down....")
			cancelFunc()
			break
		case <-time.After(waitDuration):
			continue
		}
	}
}

// makeHealthChannels creates the required channels for communication with the health service
func makeHealthChannels() healthcontract.HealthChannels {
	rrc := make(chan healthcontract.ResourceHealthRegistrationMessage, ChannelBufferSize)
	hpc := make(chan healthcontract.ResourceHealthDataMessage, ChannelBufferSize)
	return healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: rrc,
		HealthToRPNotificationChannel:         hpc,
	}
}

func getArmConfig() (armauth.ArmConfig, error) {
	var arm armauth.ArmConfig
	var err error
	skipARM, ok := os.LookupEnv("SKIP_ARM")
	if ok && strings.EqualFold(skipARM, "true") {
		arm = armauth.ArmConfig{}
	} else {
		arm, err = armauth.GetArmConfig()
	}

	return arm, err
}
