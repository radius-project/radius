// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armauth

import (
	"context"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/sdk"
	ucpsp "github.com/project-radius/radius/pkg/ucp/secret/provider"
)

// Authentication methods
const (
	UCPCredentialsAuth   = "UCPCredentialAuth"
	ServicePrincipalAuth = "ServicePrincipal"
	ManagedIdentityAuth  = "ManagedIdentity"
	CliAuth              = "CLI"
)

// ArmConfig is the configuration we use for managing ARM resources
type ArmConfig struct {
	// ClientOptions is the client options for Azure SDK client.
	ClientOptions clientv2.Options
}

// Init initializes the clients in ArmConfig.
func (ac *ArmConfig) Init(ctx context.Context) error {
	switch cli := ac.ClientOptions.Cred.(type) {
	case *aztoken.UCPCredential:
		cli.StartCredentialRotater(ctx)
	}

	return nil
}

// Options represents the options of ArmConfig.
type Options struct {
	// SecretProvider is the provider to get the secret client.
	SecretProvider *ucpsp.SecretProvider

	// UCPConnection is a connection to the UCP endpoint.
	UCPConnection sdk.Connection
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
	case UCPCredentialsAuth:
		return aztoken.NewUCPCredential(opt.SecretProvider, opt.UCPConnection)
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
