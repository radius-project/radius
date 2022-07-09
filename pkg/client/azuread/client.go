// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azuread

import (
	"fmt"
	"os"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

type AuthMethod string

// Authentication methods
const (
	ServicePrincipalAuth AuthMethod = "ServicePrincipal"
	ManagedIdentityAuth  AuthMethod = "ManagedIdentity"
	CliAuth              AuthMethod = "CLI"
)

// Options is the configuration we use for managing ARM resources
type Options struct {
	// Instance represents Azure AD endpoint.
	Instance string `yaml:"instance,omitempty"`
	// ClientID represents Service Principal or Azure AD application ID.
	ClientID string `yaml:"clientId"`
	// TenantID represents Tenant ID of servie principal.
	TenantID string `yaml:"tenantId"`
	// ClientSecret represents the client secret of ClientID.
	ClientSecret string `yaml:"clientSecret,omitempty"`
}

// GetAuthorizer returns an ARM authorizer and the client ID for the current process
func GetAuthorizer(opts *Options) (autorest.Authorizer, error) {
	authMethod := opts.AuthenticationMethod()

	var auth autorest.Authorizer
	var err error
	if authMethod == ServicePrincipalAuth {
		auth, err = opts.authServicePrincipal()
	} else if authMethod == ManagedIdentityAuth {
		auth, err = opts.authMSI()
	} else {
		auth, err = opts.authCLI()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to authorize with auth type %q: %w", authMethod, err)
	}

	return auth, nil
}

func (i *Options) authServicePrincipal() (autorest.Authorizer, error) {
	clientcfg := auth.NewClientCredentialsConfig(i.ClientID, i.ClientSecret, i.TenantID)
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

func (i *Options) authMSI() (autorest.Authorizer, error) {
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

func (i *Options) authCLI() (autorest.Authorizer, error) {
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

// AuthMethod returns the authentication method used by the RP
func (i *Options) AuthenticationMethod() AuthMethod {
	if i.ClientID != "" && i.ClientSecret != "" && i.TenantID != "" {
		return ServicePrincipalAuth
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
