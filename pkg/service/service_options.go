// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package service

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/healthcontract"
	"go.mongodb.org/mongo-driver/mongo"
	k8sClient "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Options are the parameters for different Radius services
type Options struct {
	Arm            armauth.ArmConfig
	K8sClient      *client.Client
	K8sClientSet   *k8sClient.Clientset
	DBClient       *mongo.Client
	DBName         string
	HealthChannels healthcontract.HealthChannels
}
