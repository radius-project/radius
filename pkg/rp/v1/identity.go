// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

import "errors"

// IdentitySettingKind represents the kind of identity setting.
type IdentitySettingKind string

const (
	// IdentityNone represents unknown identity.
	IdentityNone IdentitySettingKind = "None"
	// AzureIdentityWorkload represents Azure Workload identity.
	AzureIdentityWorkload IdentitySettingKind = "azure.com.workload"
)

// IdentitySettings represents the identity info to access azure resource, such as Key vault.
type IdentitySettings struct {
	// Kind represents the type of authentication.
	Kind IdentitySettingKind `json:"kind"`
	// OIDCIssuer represents the name of OIDC issuer.
	OIDCIssuer string `json:"oidcIssuer,omitempty"`
	// Resource represents the resource id of managed identity.
	Resource string `json:"resource,omitempty"`
}

// Validate validates IdentitySettings.
func (is *IdentitySettings) Validate() error {
	if is == nil {
		return nil
	}

	if is.Kind == AzureIdentityWorkload {
		if is.OIDCIssuer == "" {
			return errors.New(".properties.oidcIssuer is required for workload identity")
		}
		if is.Resource != "" {
			return errors.New(".properties.resource is read-only property")
		}
	}
	return nil
}
