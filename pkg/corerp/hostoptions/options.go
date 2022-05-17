// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

type CloudEnvironmentType string

const (
	AzureDogfood      CloudEnvironmentType = "Dogfood"
	AzureCloud        CloudEnvironmentType = "AzureCloud"
	AzureChinaCloud   CloudEnvironmentType = "AzureChinaCloud"
	AzureUSGovernment CloudEnvironmentType = "AzureUSGovernment"
	AzureGermanCloud  CloudEnvironmentType = "AzureGermanCloud"
)

type AuthentificationType string

const (
	ClientCertificateAuthType AuthentificationType = "ClientCertificate"
	AADPoPAuthType            AuthentificationType = "PoP"
)

// CloudEnvironmentOptions represents the cloud environment.
type CloudEnvironmentOptions struct {
	Name         CloudEnvironmentType `yaml:"name"`
	RoleLocation string               `yaml:"roleLocation"`
}

// IdentityOptions includes authentication options to issue JWT from Azure AD.
type IdentityOptions struct {
	ClientID    string `yaml:"clientId"`
	Instance    string `yaml:"instance"`
	TenantID    string `yaml:"tenantId"`
	ArmEndpoint string `yaml:"armEndpoint"`
	Audience    string `yaml:"audience"`
	PemCertPath string `yaml:"pemCertPath"`
}
