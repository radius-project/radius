// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hostoptions

type EnvironmentType string

const (
	Development       EnvironmentType = "Dev"
	SelfHosted        EnvironmentType = "Self-Hosted"
	AzureDogfood      EnvironmentType = "Dogfood"
	AzureCloud        EnvironmentType = "AzureCloud"
	AzureChinaCloud   EnvironmentType = "AzureChinaCloud"
	AzureUSGovernment EnvironmentType = "AzureUSGovernment"
	AzureGermanCloud  EnvironmentType = "AzureGermanCloud"
)

type AuthentificationType string

const (
	ClientCertificateAuthType AuthentificationType = "ClientCertificate"
	AADPoPAuthType            AuthentificationType = "PoP"
)

// EnvironmentOptions represents the environment.
type EnvironmentOptions struct {
	Name         EnvironmentType `yaml:"name"`
	RoleLocation string          `yaml:"roleLocation"`
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
