// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

// ServicePrincipal specifies the properties of an Azure service principal
type ServicePrincipal struct {
	ClientID     string
	ClientSecret string
	TenantID     string
}
