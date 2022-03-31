// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package providers

// Providers supported by Radius
// The RP will be able to support a resource only if the corresponding provider is configured with the RP
const (
	ProviderAzure = "azure"
	// This is a special case for support AAD Pod Identity which is not an ARM resource but a modification of an AKS Cluster
	ProviderAzureKubernetesService = "aks"
	ProviderKubernetes             = "kubernetes"
)
