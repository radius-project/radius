// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package backend

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/connectorrp/backend/controller"
)

const (
	providerName = "Applications.Connector"
)

// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	worker.Service
}

// NewService creates new service instance to run AsyncReqeustProcessWorker.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			ProviderName: providerName,
			Options:      options,
		},
	}
}

// Name represents the service name.
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", providerName)
}

// Run starts the service and worker.
func (w *Service) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
	}

	for _, rtName := range controller.ResourceTypeNames {
		// register put
		err := w.Controllers.Register(
			ctx,
			rtName,
			v1.OperationPut,
			controller.NewCreateOrUpdateResource,
			w.DeploymentProcessors["connector-rp"])
		if err != nil {
			panic(err)
		}
		// register patch
		err = w.Controllers.Register(
			ctx,
			rtName,
			v1.OperationPatch,
			controller.NewCreateOrUpdateResource,
			w.DeploymentProcessors["connector-rp"])
		if err != nil {
			panic(err)
		}
		// register delete
		err = w.Controllers.Register(
			ctx,
			rtName,
			v1.OperationDelete,
			controller.NewDeleteResource,
			w.DeploymentProcessors["connector-rp"])
		if err != nil {
			panic(err)
		}
	}

	return w.Start(ctx, worker.Options{})
}
