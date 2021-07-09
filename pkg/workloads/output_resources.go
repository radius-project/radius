// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package workloads

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// OutputResource represents the output of rendering a resource
type OutputResource struct {
	Resource           interface{}
	Deployed           bool   // TODO: Temporary workaround till some resources are deployed in Render phase
	LocalID            string // Resources need to be tracked even before actually creating them. Local ID provides a way to track them.
	Managed            bool
	ResourceKind       string
	OutputResourceType string
	OutputResourceInfo interface{}
}

// ARMInfo contains the details of an output ARM resource
type ARMInfo struct {
	ResourceID   string
	ResourceType string
	APIVersion   string
}

// K8sInfo contains the details of an output Kubernetes resource
type K8sInfo struct {
	Kind       string
	APIVersion string
	Name       string
	Namespace  string
}

// AADPodIdentity contains the details of an output AAD Pod Identity resource
type AADPodIdentity struct {
	AKSClusterName string
	Name           string
	Namespace      string
}

// NewKubernetesResource creates a Kubernetes WorkloadResource
func NewKubernetesResource(localID string, obj runtime.Object) OutputResource {
	return OutputResource{ResourceKind: ResourceKindKubernetes, LocalID: localID, Resource: obj}
}

func (wr OutputResource) IsKubernetesResource() bool {
	return wr.ResourceKind == ResourceKindKubernetes
}

// Represents local IDs used for output resources
const (
	LocalIDAzureCosmosDBMongo            = "AzureCosmosDBMongo"
	LocalIDDaprStateStoreAzureStorage    = "DaprStateStoreAzureStorage"
	LocalIDDaprStateStoreSQLServer       = "DaprStateStoreSQLServer"
	LocalIDKeyVault                      = "KeyVault"
	LocalIDKeyVaultSecret                = "KeyVaultSecret"
	LocalIDDeployment                    = "Deployment"
	LocalIDService                       = "Service"
	LocalIDUserAssignedManagedIdentityKV = "UserAssignedManagedIdentity-KV"
	LocalIDRoleAssignmentKVKeys          = "RoleAssignment-KVKeys"
	LocalIDRoleAssignmentKVSecretsCerts  = "RoleAssignment-KVSecretsCerts"
	LocalIDAADPodIdentity                = "AADPodIdentity"
	LocalIDAzureServiceBusTopic          = "AzureServiceBusTopic"
	LocalIDAzureServiceBusQueue          = "AzureServiceBusQueue"
	LocalIDAzureCosmosDBSQL              = "AzureCosmosDBSQL"
)
