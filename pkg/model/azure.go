// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/containerv1alpha1"
	"github.com/Azure/radius/pkg/workloads/cosmosdbmongov1alpha1"
	"github.com/Azure/radius/pkg/workloads/cosmosdbsqlv1alpha1"
	"github.com/Azure/radius/pkg/workloads/dapr"
	"github.com/Azure/radius/pkg/workloads/daprpubsubv1alpha1"
	"github.com/Azure/radius/pkg/workloads/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/workloads/inboundroute"
	"github.com/Azure/radius/pkg/workloads/keyvaultv1alpha1"
	"github.com/Azure/radius/pkg/workloads/mongodbv1alpha1"
	"github.com/Azure/radius/pkg/workloads/redisv1alpha1"
	"github.com/Azure/radius/pkg/workloads/servicebusqueuev1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAzureModel(arm armauth.ArmConfig, k8s *client.Client) ApplicationModel {
	renderers := map[string]workloads.WorkloadRenderer{
		daprstatestorev1alpha1.Kind:  &daprstatestorev1alpha1.Renderer{StateStores: daprstatestorev1alpha1.SupportedAzureStateStoreKindValues},
		daprpubsubv1alpha1.Kind:      &daprpubsubv1alpha1.Renderer{},
		cosmosdbmongov1alpha1.Kind:   &cosmosdbmongov1alpha1.Renderer{Arm: arm},
		cosmosdbsqlv1alpha1.Kind:     &cosmosdbsqlv1alpha1.Renderer{Arm: arm},
		mongodbv1alpha1.Kind:         &mongodbv1alpha1.AzureRenderer{Arm: arm},
		containerv1alpha1.Kind:       &inboundroute.Renderer{Inner: &dapr.Renderer{Inner: &containerv1alpha1.Renderer{Arm: arm}}},
		servicebusqueuev1alpha1.Kind: &servicebusqueuev1alpha1.Renderer{Arm: arm},
		keyvaultv1alpha1.Kind:        &keyvaultv1alpha1.Renderer{Arm: arm},
		redisv1alpha1.Kind:           &redisv1alpha1.AzureRenderer{Arm: arm},
	}
	handlers := map[string]handlers.ResourceHandler{
		outputresource.KindKubernetes:                       handlers.NewKubernetesHandler(*k8s),
		outputresource.KindDaprStateStoreAzureStorage:       handlers.NewDaprStateStoreAzureStorageHandler(arm, *k8s),
		outputresource.KindDaprStateStoreSQLServer:          handlers.NewDaprStateStoreSQLServerHandler(arm, *k8s),
		outputresource.KindDaprPubSubTopicAzureServiceBus:   handlers.NewDaprPubSubServiceBusHandler(arm, *k8s),
		outputresource.KindAzureCosmosDBMongo:               handlers.NewAzureCosmosDBMongoHandler(arm),
		outputresource.KindAzureCosmosDBSQL:                 handlers.NewAzureCosmosDBSQLHandler(arm),
		outputresource.KindAzureServiceBusQueue:             handlers.NewAzureServiceBusQueueHandler(arm),
		outputresource.KindAzureKeyVault:                    handlers.NewAzureKeyVaultHandler(arm),
		outputresource.KindAzurePodIdentity:                 handlers.NewAzurePodIdentityHandler(arm),
		outputresource.KindAzureUserAssignedManagedIdentity: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		outputresource.KindAzureRoleAssignment:              handlers.NewAzureRoleAssignmentHandler(arm),
		outputresource.KindAzureKeyVaultSecret:              handlers.NewAzureKeyVaultSecretHandler(arm),
		outputresource.KindAzureRedis:                       handlers.NewAzureRedisHandler(arm),
	}
	return NewModel(renderers, handlers)
}
