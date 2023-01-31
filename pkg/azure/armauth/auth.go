// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armauth

import (
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	sdk "github.com/project-radius/radius/pkg/sdk/credentials"
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
	CredentialProvider sdk.CredentialProvider[sdk.AzureCredential]
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
		return aztoken.NewUCPCredential(aztoken.UCPCredentialOptions{
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

	settings, err := auth.GetSettingsFromEnvironment()
	if err == nil && settings.Values[auth.ClientID] != "" && settings.Values[auth.ClientSecret] != "" {
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
