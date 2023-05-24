/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
