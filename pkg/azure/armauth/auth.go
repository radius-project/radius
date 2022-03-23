// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armauth

import (
	"errors"
	"os"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

// Authentication methods
const (
	ServicePrincipalAuth = "ServicePrincipal"
	ManagedIdentityAuth  = "ManagedIdentity"
	CliAuth              = "CLI"
)

// ArmConfig is the configuration we use for managing ARM resources
type ArmConfig struct {
	Auth              autorest.Authorizer
	SubscriptionID    string
	ResourceGroup     string
	K8sSubscriptionID string
	K8sResourceGroup  string
	K8sClusterName    string
}

// GetArmConfig gets the configuration we use for managing ARM resources
func GetArmConfig() (*ArmConfig, error) {
	auth, err := GetArmAuthorizer()
	if err != nil {
		return &ArmConfig{}, err
	}

	subscriptionID := os.Getenv("ARM_SUBSCRIPTION_ID")
	if subscriptionID == "" {
		return &ArmConfig{}, errors.New("required env-var ARM_SUBSCRIPTION_ID is missing")
	}

	resourceGroup := os.Getenv("ARM_RESOURCE_GROUP")
	if resourceGroup == "" {
		return &ArmConfig{}, errors.New("required env-var ARM_RESOURCE_GROUP is missing")
	}

	return &ArmConfig{
		Auth:           auth,
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,

		K8sSubscriptionID: os.Getenv("K8S_SUBSCRIPTION_ID"),
		K8sResourceGroup:  os.Getenv("K8S_RESOURCE_GROUP"),
		K8sClusterName:    os.Getenv("K8S_CLUSTER_NAME"),
	}, nil
}

// GetArmAuthorizer returns an ARM authorizer and the client ID for the current process
func GetArmAuthorizer() (autorest.Authorizer, error) {
	authMethod := GetAuthMethod()
	if authMethod == ServicePrincipalAuth {
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
	} else if authMethod == ManagedIdentityAuth {
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
	} else {
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
}

// GetAuthMethod returns the authentication method used by the RP
func GetAuthMethod() string {
	// Allow explicit configuration of the auth method, and fall back
	// to auto-detection if unspecified
	authMethod := os.Getenv("ARM_AUTH_METHOD")
	switch authMethod {
	case CliAuth:
	case ManagedIdentityAuth:
	case ServicePrincipalAuth:
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
