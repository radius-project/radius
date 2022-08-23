// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package health

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/rp/hostoptions"
	"go.mongodb.org/mongo-driver/mongo"
	"k8s.io/client-go/rest"
)

type ServiceOptions struct {
	Arm             *armauth.ArmConfig
	DBClientFactory func(ctx context.Context) (*mongo.Database, error)
	HealthChannels  healthcontract.HealthChannels
	K8sConfig       *rest.Config
}

func NewServiceOptions(options hostoptions.HostOptions) ServiceOptions {
	return ServiceOptions{
		Arm:             options.Arm,
		DBClientFactory: options.DBClientFactory,
		HealthChannels:  options.HealthChannels,
		K8sConfig:       options.K8sConfig,
	}
}
