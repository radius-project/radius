/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backend

import (
	"context"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	dapr_dm "github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/daprrp/processors/pubsubbrokers"
	"github.com/radius-project/radius/pkg/daprrp/processors/secretstores"
	"github.com/radius-project/radius/pkg/daprrp/processors/statestores"
	ds_dm "github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	mongo_prc "github.com/radius-project/radius/pkg/datastoresrp/processors/mongodatabases"
	redis_prc "github.com/radius-project/radius/pkg/datastoresrp/processors/rediscaches"
	sql_prc "github.com/radius-project/radius/pkg/datastoresrp/processors/sqldatabases"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/linkrp"
	"github.com/radius-project/radius/pkg/linkrp/datamodel"
	"github.com/radius-project/radius/pkg/linkrp/frontend/handler"
	"github.com/radius-project/radius/pkg/linkrp/processors"
	"github.com/radius-project/radius/pkg/linkrp/processors/daprpubsubbrokers"
	"github.com/radius-project/radius/pkg/linkrp/processors/daprsecretstores"
	"github.com/radius-project/radius/pkg/linkrp/processors/daprstatestores"
	"github.com/radius-project/radius/pkg/linkrp/processors/extenders"
	"github.com/radius-project/radius/pkg/linkrp/processors/mongodatabases"
	"github.com/radius-project/radius/pkg/linkrp/processors/rabbitmqmessagequeues"
	"github.com/radius-project/radius/pkg/linkrp/processors/rediscaches"
	"github.com/radius-project/radius/pkg/linkrp/processors/sqldatabases"
	msg_dm "github.com/radius-project/radius/pkg/messagingrp/datamodel"
	"github.com/radius-project/radius/pkg/messagingrp/processors/rabbitmqqueues"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/sdk/clients"
	"k8s.io/client-go/discovery"

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	backend_ctrl "github.com/radius-project/radius/pkg/linkrp/backend/controller"

	"github.com/radius-project/radius/pkg/ucp/secret/provider"
)

type Service struct {
	worker.Service
}

// NewService creates a new Service instance with the given options.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			Options:      options,
			ProviderName: handler.LinkProviderNamespace,
		},
	}
}

// Name returns a string containing the namespace of the LinkProvider.
func (s *Service) Name() string {
	return fmt.Sprintf("%s async worker", handler.LinkProviderNamespace)
}

// Run initializes the service and registers controllers for each resource type to handle create/update/delete operations.
func (s *Service) Run(ctx context.Context) error {
	if err := s.Init(ctx); err != nil {
		return err
	}

	runtimeClient, err := kubeutil.NewRuntimeClient(s.Options.K8sConfig)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(s.Options.K8sConfig)
	if err != nil {
		return err
	}

	// Use legacy discovery client to avoid the issue of the staled GroupVersion discovery(api.ucp.dev/v1alpha3).
	// TODO: Disable UseLegacyDiscovery once https://github.com/radius-project/radius/issues/5974 is resolved.
	discoveryClient.UseLegacyDiscovery = true

	client := processors.NewResourceClient(s.Options.Arm, s.Options.UCPConnection, runtimeClient, discoveryClient)
	clientOptions := sdk.NewClientOptions(s.Options.UCPConnection)

	deploymentEngineClient, err := clients.NewResourceDeploymentsClient(&clients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          s.Options.UCPConnection.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(s.Options.UCPConnection),
	})
	if err != nil {
		return err
	}

	configLoader := configloader.NewEnvironmentLoader(clientOptions)
	engine := engine.NewEngine(engine.Options{
		ConfigurationLoader: configLoader,
		Drivers: map[string]driver.Driver{
			recipes.TemplateKindBicep: driver.NewBicepDriver(clientOptions, deploymentEngineClient, client),
			recipes.TemplateKindTerraform: driver.NewTerraformDriver(s.Options.UCPConnection, provider.NewSecretProvider(s.Options.Config.SecretProvider),
				driver.TerraformOptions{
					Path: s.Options.Config.Terraform.Path,
				}, s.KubeClientSet),
		},
	})

	// resourceTypes is the array that holds resource types that needs async processing.
	// We use this array to register backend controllers for each resource.
	resourceTypes := []struct {
		TypeName            string
		CreatePutController func(options ctrl.Options) (ctrl.Controller, error)
	}{
		{linkrp.MongoDatabasesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &mongodatabases.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.MongoDatabase, datamodel.MongoDatabase](processor, engine, client, configLoader, options)
		}},
		{linkrp.RedisCachesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &rediscaches.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.RedisCache, datamodel.RedisCache](processor, engine, client, configLoader, options)
		}},
		{linkrp.SqlDatabasesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &sqldatabases.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.SqlDatabase, datamodel.SqlDatabase](processor, engine, client, configLoader, options)
		}},
		{linkrp.RabbitMQMessageQueuesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &rabbitmqmessagequeues.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.RabbitMQMessageQueue, datamodel.RabbitMQMessageQueue](processor, engine, client, configLoader, options)
		}},
		{linkrp.DaprStateStoresResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprstatestores.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.DaprStateStore, datamodel.DaprStateStore](processor, engine, client, configLoader, options)
		}},
		{linkrp.DaprSecretStoresResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprsecretstores.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.DaprSecretStore, datamodel.DaprSecretStore](processor, engine, client, configLoader, options)
		}},
		{linkrp.DaprPubSubBrokersResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprpubsubbrokers.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.DaprPubSubBroker, datamodel.DaprPubSubBroker](processor, engine, client, configLoader, options)
		}},
		{linkrp.ExtendersResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &extenders.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.Extender, datamodel.Extender](processor, engine, client, configLoader, options)
		}},

		// Updates for Spliting Linkrp Namespace
		{linkrp.N_RabbitMQQueuesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &rabbitmqqueues.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*msg_dm.RabbitMQQueue, msg_dm.RabbitMQQueue](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_DaprStateStoresResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &statestores.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprStateStore, dapr_dm.DaprStateStore](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_DaprSecretStoresResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &secretstores.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprSecretStore, dapr_dm.DaprSecretStore](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_DaprPubSubBrokersResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &pubsubbrokers.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprPubSubBroker, dapr_dm.DaprPubSubBroker](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_MongoDatabasesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &mongo_prc.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.MongoDatabase, ds_dm.MongoDatabase](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_RedisCachesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &redis_prc.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.RedisCache, ds_dm.RedisCache](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_SqlDatabasesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &sql_prc.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.SqlDatabase, ds_dm.SqlDatabase](processor, engine, client, configLoader, options)
		}},
	}

	opts := ctrl.Options{
		DataProvider: s.StorageProvider,
		KubeClient:   s.KubeClient,
	}

	for _, rt := range resourceTypes {
		// Register controllers
		err = s.Controllers.Register(ctx, rt.TypeName, v1.OperationDelete, func(options ctrl.Options) (ctrl.Controller, error) {
			return backend_ctrl.NewDeleteResource(options, engine)
		}, opts)
		if err != nil {
			return err
		}
		err = s.Controllers.Register(ctx, rt.TypeName, v1.OperationPut, rt.CreatePutController, opts)
		if err != nil {
			return err
		}
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
