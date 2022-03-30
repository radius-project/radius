// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package frontend

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/radrp/hostoptions"
	"go.mongodb.org/mongo-driver/mongo"
	"k8s.io/client-go/rest"
)

type ServiceOptions struct {
	Address         string
	Arm             *armauth.ArmConfig
	Authenticate    bool
	BasePath        string
	DBClientFactory func(ctx context.Context) (*mongo.Database, error)
	HealthChannels  healthcontract.HealthChannels
	K8sConfig       *rest.Config
	TLSCertDir      string
}

func NewServiceOptions(options hostoptions.HostOptions) ServiceOptions {
	return ServiceOptions{
		Address:         options.Address,
		Arm:             options.Arm,
		Authenticate:    options.Authenticate,
		BasePath:        options.BasePath,
		DBClientFactory: options.DBClientFactory,
		HealthChannels:  options.HealthChannels,
		K8sConfig:       options.K8sConfig,
		TLSCertDir:      options.TLSCertDir,
	}
}
