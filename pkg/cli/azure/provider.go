// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

// Provider specifies the properties required to configure Azure provider for cloud resources
type Provider struct {
	SubscriptionID   string
	ResourceGroup    string
	ServicePrincipal *ServicePrincipal
}
