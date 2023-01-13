// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armauth

import (
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
)

// Authentication methods
const (
	ServicePrincipalAuth = "ServicePrincipal"
	ManagedIdentityAuth  = "ManagedIdentity"
	CliAuth              = "CLI"
)

// ArmConfig is the configuration we use for managing ARM resources
type ArmConfig struct {
	// Auth is the old azure client authenticator.
	// TODO: Migrate authenticator and clients to new azure sdk - https://github.com/project-radius/radius/issues/4268
	Auth autorest.Authorizer

	// ClientOptions is the client v2 options including new client credentials.
	ClientOptions clientv2.Options
}

// GetArmConfig gets the configuration we use for managing ARM resources
func GetArmConfig() (*ArmConfig, error) {
	auth, err := GetArmAuthorizer()
	if err != nil {
		return &ArmConfig{}, err
	}

	// Create Client v2 Credential object.
	cred, err := NewARMCredential()
	if err != nil {
		return nil, err
	}

	return &ArmConfig{
		Auth:          auth,
		ClientOptions: clientv2.Options{Cred: cred},
	}, nil
}

// GetArmAuthorizerFromValues returns an ARM authorizer and the client ID for the current process from the provided service principal values
func GetArmAuthorizerFromValues(clientID string, clientSecret string, tenantID string) (autorest.Authorizer, error) {
	clientcfg := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID)
	auth, err := clientcfg.Authorizer()
	if err != nil {
		return nil, err
	}

	token, err := clientcfg.ServicePrincipalToken()
	if err != nil {
		return nil, err
	}

	err = token.EnsureFresh()
	if err != nil {
		return nil, err
	}

	return auth, nil
}

// NewARMCredential returns new azure client credential
func NewARMCredential() (azcore.TokenCredential, error) {
	authMethod := GetAuthMethod()

	if authMethod == ServicePrincipalAuth {
		return azidentity.NewClientSecretCredential(
			os.Getenv("AZURE_TENANT_ID"),
			os.Getenv("AZURE_CLIENT_ID"),
			os.Getenv("AZURE_CLIENT_SECRET"), nil)
	} else if authMethod == ManagedIdentityAuth {
		return azidentity.NewManagedIdentityCredential(nil)
	} else {
		return azidentity.NewAzureCLICredential(nil)
	}
}

// GetArmAuthorizer returns an ARM authorizer and the client ID for the current process
func GetArmAuthorizer() (autorest.Authorizer, error) {
	authMethod := GetAuthMethod()

	var auth autorest.Authorizer
	var err error
	if authMethod == ServicePrincipalAuth {
		auth, err = authServicePrincipal()
	} else if authMethod == ManagedIdentityAuth {
		auth, err = authMSI()
	} else {
		auth, err = authCLI()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to authorize with auth type %q: %w", authMethod, err)
	}

	return auth, nil
}

func authServicePrincipal() (autorest.Authorizer, error) {
	clientcfg := auth.NewClientCredentialsConfig(os.Getenv("AZURE_CLIENT_ID"), os.Getenv("AZURE_CLIENT_SECRET"), os.Getenv("AZURE_TENANT_ID"))
	auth, err := clientcfg.Authorizer()
	if err != nil {
		return nil, err
	}

	token, err := clientcfg.ServicePrincipalToken()
	if err != nil {
		return nil, err
	}

	err = token.EnsureFresh()
	if err != nil {
		return nil, err
	}

	return auth, nil
}

func authMSI() (autorest.Authorizer, error) {
	config := auth.NewMSIConfig()
	token, err := config.ServicePrincipalToken()
	if err != nil {
		return nil, err
	}

	err = token.EnsureFresh()
	if err != nil {
		return nil, err
	}

	auth, err := config.Authorizer()
	if err != nil {
		return nil, err
	}

	return auth, nil
}

func authCLI() (autorest.Authorizer, error) {
	settings, err := auth.GetSettingsFromEnvironment()
	if err != nil {
		return nil, err
	}

	auth, err := auth.NewAuthorizerFromCLIWithResource(settings.Environment.ResourceManagerEndpoint)

	if err != nil {
		return nil, err
	}

	return auth, nil
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
