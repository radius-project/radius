// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

// Provider specifies the properties required to configure Azure provider for cloud resources
type Provider struct {
	SubscriptionID      string
	ResourceGroup       string
	ServicePrincipal    *ServicePrincipal
	PodIdentitySelector *string

	AKS *AKSConfig
}

// AKSConfig is configuration to link the RP to an AKS cluster. This is used for managing pod identities in Azure. This will go away when we support
// BYO-cluster via the environment.
type AKSConfig struct {
	SubscriptionID string
	ResourceGroup  string
	ClusterName    string
}
