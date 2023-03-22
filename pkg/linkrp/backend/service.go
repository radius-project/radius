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
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/frontend/handler"
	"github.com/project-radius/radius/pkg/linkrp/model"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	"github.com/project-radius/radius/pkg/recipes/driver"
	"github.com/project-radius/radius/pkg/recipes/engine"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
	"github.com/project-radius/radius/pkg/sdk"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	backend_ctrl "github.com/project-radius/radius/pkg/linkrp/backend/controller"
)

var (
	// ResourceTypeNames is the array that holds resource types that needs async processing.
	// We use this array to generate generic backend controller for each resource.
	ResourceTypeNames = []string{
		linkrp.MongoDatabasesResourceType,
		linkrp.RedisCachesResourceType,
		linkrp.DaprStateStoresResourceType,
	}
)

type Service struct {
	worker.Service
}

func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			Options:      options,
			ProviderName: handler.ProviderNamespaceName,
		},
	}
}

func (s *Service) Name() string {
	return fmt.Sprintf("%s async worker", handler.ProviderNamespaceName)
}

func (s *Service) Run(ctx context.Context) error {
	if err := s.Init(ctx); err != nil {
		return err
	}

	linkAppModel, err := model.NewApplicationModel(s.Options.Arm, s.KubeClient, s.Options.UCPConnection)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	ucpClientOptions := sdk.NewClientOptions(s.Options.UCPConnection)

	client, err := GetUCPDeploymentClient(s.Options.UCPConnection)
	if err != nil {
		return fmt.Errorf("failed to initialize UCP deployment client: %w", err)
	}

	loader := &configloader.EnvironmentLoader{UCPClientOptions: ucpClientOptions}
	engine := engine.NewEngine(engine.Options{
		ConfigurationLoader: loader,
		Drivers: map[string]recipes.Driver{
			"bicep": &driver.Driver{UCPClientOptions: ucpClientOptions, DeploymentClient: client},
		},
	})

	opts := ctrl.Options{
		DataProvider: s.StorageProvider,
		KubeClient:   s.KubeClient,
		GetLinkDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewDeploymentProcessor(linkAppModel, s.StorageProvider, sv.NewSecretValueClient(s.Options.Arm), s.KubeClient)
		},
	}

	for _, rt := range ResourceTypeNames {
		// Register controllers
		err = s.Controllers.Register(ctx, rt, v1.OperationDelete, backend_ctrl.NewDeleteResource, opts)
		if err != nil {
			panic(err)
		}
		err = s.Controllers.Register(ctx, rt, v1.OperationPut, backend_ctrl.NewCreateOrUpdateResource, opts)
		if err != nil {
			panic(err)
		}
	}
	err = s.Controllers.Register(ctx, "Applications.Link/redisCaches", v1.OperationPut, func(opts ctrl.Options) (ctrl.Controller, error) {
		return backend_ctrl.NewCreateOrUpdateRefactor(opts, engine, s.Options.Arm)
	}, opts)
	if err != nil {
		return err
	}
	workerOpts := worker.Options{}
	if s.Options.Config.WorkerServer != nil {
		if s.Options.Config.WorkerServer.MaxOperationConcurrency != nil {
			workerOpts.MaxOperationConcurrency = *s.Options.Config.WorkerServer.MaxOperationConcurrency
		}
		if s.Options.Config.WorkerServer.MaxOperationRetryCount != nil {
			workerOpts.MaxOperationRetryCount = *s.Options.Config.WorkerServer.MaxOperationRetryCount
		}
	}

	return s.Start(ctx, workerOpts)
}
