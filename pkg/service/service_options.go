// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package service

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/healthcontract"
	"go.mongodb.org/mongo-driver/mongo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Options are the parameters for different Radius services
type Options struct {
	Arm            armauth.ArmConfig
	K8s            *client.Client
	DBClient       *mongo.Client
	DBName         string
	HealthChannels healthcontract.HealthChannels
}
