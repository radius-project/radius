/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package armauth

import (
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	azcred "github.com/project-radius/radius/pkg/azure/credential"
	sdk_cred "github.com/project-radius/radius/pkg/ucp/credentials"
)

// Authentication methods
const (
	UCPCredentialAuth    = "UCPCredential"
	ServicePrincipalAuth = "ServicePrincipal"
	ManagedIdentityAuth  = "ManagedIdentity"
	CliAuth              = "CLI"
)

// ArmConfig is the configuration we use for managing ARM resources
type ArmConfig struct {
	// ClientOptions is the client options for Azure SDK client.
	ClientOptions clientv2.Options
}

// Options represents the options of ArmConfig.
type Options struct {
	// CredentialProvider is an UCP credential client for Azure service principal.
	CredentialProvider sdk_cred.CredentialProvider[sdk_cred.AzureCredential]
}

// NewArmConfig gets the configuration we use for managing ARM resources
func NewArmConfig(opt *Options) (*ArmConfig, error) {
	if opt == nil {
		opt = &Options{}
	}

	cred, err := NewARMCredential(opt)
	if err != nil {
		return nil, err
	}

	return &ArmConfig{
		ClientOptions: clientv2.Options{Cred: cred},
	}, nil
}

// NewARMCredential returns new azure client credential
func NewARMCredential(opt *Options) (azcore.TokenCredential, error) {
	authMethod := GetAuthMethod()

	switch authMethod {
	case UCPCredentialAuth:
		return azcred.NewUCPCredential(azcred.UCPCredentialOptions{
			Provider: opt.CredentialProvider,
		})
	case ServicePrincipalAuth:
		return azidentity.NewEnvironmentCredential(nil)
	case ManagedIdentityAuth:
		return azidentity.NewManagedIdentityCredential(nil)
	default:
		return azidentity.NewAzureCLICredential(nil)
	}
}

// GetAuthMethod returns the authentication method used by the RP
func GetAuthMethod() string {
	// Allow explicit configuration of the auth method, and fall back
	// to auto-detection if unspecified
	authMethod := os.Getenv("ARM_AUTH_METHOD")
	if authMethod != "" {
		return authMethod
	}

	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")

	if clientID != "" && clientSecret != "" {
		return ServicePrincipalAuth
	} else if os.Getenv("MSI_ENDPOINT") != "" || os.Getenv("IDENTITY_ENDPOINT") != "" {
		return ManagedIdentityAuth
	} else {
		return CliAuth
	}
}

// IsServicePrincipalConfigured determines whether a service principal is specifed
func IsServicePrincipalConfigured() (bool, error) {
	return GetAuthMethod() == ServicePrincipalAuth, nil
}
