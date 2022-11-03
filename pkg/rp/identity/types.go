// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package identity

// IdentitySettingKind represents the kind of identity setting.
type IdentitySettingKind string

const (
	// IdentityNone represents unknown identity.
	IdentityNone IdentitySettingKind = "None"
	// AzureIdentityWorkload represents Azure Workload identity.
	AzureIdentityWorkload IdentitySettingKind = "azure.com.workload"
	// AzureIdentitySystemAssigned represents System assigned identity.
	AzureIdentitySystemAssigned IdentitySettingKind = "azure.com.systemassigned"
)

// IdentitySettings represents the identity info to access azure resource, such as Key vault.
type IdentitySettings struct {
	// Kind represents the type of authentication.
	Kind IdentitySettingKind `json:"kind"`
	// Resource represents the resource id of managed identity.
	Resource string `json:"resource,omitempty"`
	// OIDCIssuer represents the name of OIDC issuer.
	OIDCIssuer string `json:"oidcIssuer,omitempty"`
}
