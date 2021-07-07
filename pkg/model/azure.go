// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/containerv1alpha1"
	"github.com/Azure/radius/pkg/workloads/cosmosdbmongov1alpha1"
	"github.com/Azure/radius/pkg/workloads/cosmosdbsqlv1alpha1"
	"github.com/Azure/radius/pkg/workloads/dapr"
	"github.com/Azure/radius/pkg/workloads/daprpubsubv1alpha1"
	"github.com/Azure/radius/pkg/workloads/daprstatestorev1alpha1"
	"github.com/Azure/radius/pkg/workloads/inboundroute"
	"github.com/Azure/radius/pkg/workloads/keyvaultv1alpha1"
	"github.com/Azure/radius/pkg/workloads/servicebusqueuev1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewAzureModel(arm armauth.ArmConfig, k8s *client.Client) ApplicationModel {
	renderers := map[string]workloads.WorkloadRenderer{
		daprstatestorev1alpha1.Kind:  &daprstatestorev1alpha1.Renderer{},
		daprpubsubv1alpha1.Kind:      &daprpubsubv1alpha1.Renderer{},
		cosmosdbmongov1alpha1.Kind:   &cosmosdbmongov1alpha1.Renderer{Arm: arm},
		cosmosdbsqlv1alpha1.Kind:     &cosmosdbsqlv1alpha1.Renderer{Arm: arm},
		containerv1alpha1.Kind:       &inboundroute.Renderer{Inner: &dapr.Renderer{Inner: &containerv1alpha1.Renderer{Arm: arm}}},
		servicebusqueuev1alpha1.Kind: &servicebusqueuev1alpha1.Renderer{Arm: arm},
		keyvaultv1alpha1.Kind:        &keyvaultv1alpha1.Renderer{Arm: arm},
	}
	handlers := map[string]handlers.ResourceHandler{
		workloads.ResourceKindKubernetes:                       handlers.NewKubernetesHandler(*k8s),
		workloads.ResourceKindDaprStateStoreAzureStorage:       handlers.NewDaprStateStoreAzureStorageHandler(arm, *k8s),
		workloads.ResourceKindDaprStateStoreSQLServer:          handlers.NewDaprStateStoreSQLServerHandler(arm, *k8s),
		workloads.ResourceKindDaprPubSubTopicAzureServiceBus:   handlers.NewDaprPubSubServiceBusHandler(arm, *k8s),
		workloads.ResourceKindAzureCosmosDBMongo:               handlers.NewAzureCosmosDBMongoHandler(arm),
		workloads.ResourceKindAzureCosmosDBSQL:                 handlers.NewAzureCosmosDBSQLHandler(arm),
		workloads.ResourceKindAzureServiceBusQueue:             handlers.NewAzureServiceBusQueueHandler(arm),
		workloads.ResourceKindAzureKeyVault:                    handlers.NewAzureKeyVaultHandler(arm),
		workloads.ResourceKindAzurePodIdentity:                 handlers.NewAzurePodIdentityHandler(arm),
		workloads.ResourceKindAzureUserAssignedManagedIdentity: handlers.NewAzureUserAssignedManagedIdentityHandler(arm),
		workloads.ResourceKindAzureRoleAssignment:              handlers.NewAzureRoleAssignmentHandler(arm),
		workloads.ResourceKindAzureKeyVaultSecret:              handlers.NewAzureKeyVaultSecretHandler(arm),
	}
	return NewModel(renderers, handlers)
}
