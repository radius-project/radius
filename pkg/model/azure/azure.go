// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha1"
	"github.com/Azure/radius/pkg/renderers/cosmosdbmongov1alpha1"
	"github.com/Azure/radius/pkg/renderers/cosmosdbsqlv1alpha1"
	"github.com/Azure/radius/pkg/renderers/dapr"
	"github.com/Azure/radius/pkg/renderers/daprpubsubv1alpha1"
	"github.com/Azure/radius/pkg/renderers/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/renderers/inboundroute"
	"github.com/Azure/radius/pkg/renderers/keyvaultv1alpha1"
	"github.com/Azure/radius/pkg/renderers/manualscale"
	"github.com/Azure/radius/pkg/renderers/mongodbv1alpha1"
	"github.com/Azure/radius/pkg/renderers/redisv1alpha1"
	"github.com/Azure/radius/pkg/renderers/servicebusqueuev1alpha1"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAzureModel(arm armauth.ArmConfig, k8s client.Client) model.ApplicationModel {
	renderers := map[string]workloads.WorkloadRenderer{
		daprstatestorev1alpha1.Kind:  &daprstatestorev1alpha1.Renderer{StateStores: daprstatestorev1alpha1.SupportedAzureStateStoreKindValues},
		daprpubsubv1alpha1.Kind:      &daprpubsubv1alpha1.Renderer{},
		cosmosdbmongov1alpha1.Kind:   &cosmosdbmongov1alpha1.Renderer{Arm: arm},
		cosmosdbsqlv1alpha1.Kind:     &cosmosdbsqlv1alpha1.Renderer{Arm: arm},
		mongodbv1alpha1.Kind:         &mongodbv1alpha1.AzureRenderer{Arm: arm},
		containerv1alpha1.Kind:       &manualscale.Renderer{Inner: &inboundroute.Renderer{Inner: &dapr.Renderer{Inner: &containerv1alpha1.Renderer{Arm: arm}}}},
		servicebusqueuev1alpha1.Kind: &servicebusqueuev1alpha1.Renderer{Arm: arm},
		keyvaultv1alpha1.Kind:        &keyvaultv1alpha1.Renderer{Arm: arm},
		redisv1alpha1.Kind:           &redisv1alpha1.AzureRenderer{Arm: arm},
	}

	handlers := map[string]model.Handlers{
		resourcekinds.KindKubernetes:                       {ResourceHandler: handlers.NewKubernetesHandler(k8s), HealthHandler: handlers.NewKubernetesHealthHandler(k8s)},
		resourcekinds.KindDaprStateStoreAzureStorage:       {ResourceHandler: handlers.NewDaprStateStoreAzureStorageHandler(arm, k8s), HealthHandler: handlers.NewDaprStateStoreAzureStorageHealthHandler(arm, k8s)},
		resourcekinds.KindDaprStateStoreSQLServer:          {ResourceHandler: handlers.NewDaprStateStoreSQLServerHandler(arm, k8s), HealthHandler: handlers.NewDaprStateStoreSQLServerHealthHandler(arm, k8s)},
		resourcekinds.KindDaprPubSubTopicAzureServiceBus:   {ResourceHandler: handlers.NewDaprPubSubServiceBusHandler(arm, k8s), HealthHandler: handlers.NewDaprPubSubServiceBusHealthHandler(arm, k8s)},
		resourcekinds.KindAzureCosmosDBMongo:               {ResourceHandler: handlers.NewAzureCosmosDBMongoHandler(arm), HealthHandler: handlers.NewAzureCosmosDBMongoHealthHandler(arm)},
		resourcekinds.KindAzureCosmosAccountMongo:          {ResourceHandler: handlers.NewAzureCosmosAccountMongoHandler(arm), HealthHandler: handlers.NewAzureCosmosAccountMongoHealthHandler(arm)},
		resourcekinds.KindAzureCosmosDBSQL:                 {ResourceHandler: handlers.NewAzureCosmosDBSQLHandler(arm), HealthHandler: handlers.NewAzureCosmosDBSQLHealthHandler(arm)},
		resourcekinds.KindAzureServiceBusQueue:             {ResourceHandler: handlers.NewAzureServiceBusQueueHandler(arm), HealthHandler: handlers.NewAzureServiceBusQueueHealthHandler(arm)},
		resourcekinds.KindAzureKeyVault:                    {ResourceHandler: handlers.NewAzureKeyVaultHandler(arm), HealthHandler: handlers.NewAzureKeyVaultHealthHandler(arm)},
		resourcekinds.KindAzurePodIdentity:                 {ResourceHandler: handlers.NewAzurePodIdentityHandler(arm), HealthHandler: handlers.NewAzurePodIdentityHealthHandler(arm)},
		resourcekinds.KindAzureUserAssignedManagedIdentity: {ResourceHandler: handlers.NewAzureUserAssignedManagedIdentityHandler(arm), HealthHandler: handlers.NewAzureUserAssignedManagedIdentityHealthHandler(arm)},
		resourcekinds.KindAzureRoleAssignment:              {ResourceHandler: handlers.NewAzureRoleAssignmentHandler(arm), HealthHandler: handlers.NewAzureRoleAssignmentHealthHandler(arm)},
		resourcekinds.KindAzureKeyVaultSecret:              {ResourceHandler: handlers.NewAzureKeyVaultSecretHandler(arm), HealthHandler: handlers.NewAzureKeyVaultSecretHealthHandler(arm)},
		resourcekinds.KindAzureRedis:                       {ResourceHandler: handlers.NewAzureRedisHandler(arm), HealthHandler: handlers.NewAzureRedisHealthHandler(arm)},
	}
	return model.NewModel(renderers, handlers)
}
