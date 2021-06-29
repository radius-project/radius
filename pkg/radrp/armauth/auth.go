// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armauth

import (
	"errors"
	"log"
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
func GetArmConfig() (ArmConfig, error) {
	auth, err := GetArmAuthorizer()
	if err != nil {
		return ArmConfig{}, err
	}

	subscriptionID := os.Getenv("ARM_SUBSCRIPTION_ID")
	if subscriptionID == "" {
		// See #565: this is temporary code that handles the case where the app resource group and control plane group are the same
		// as it was in 0.2.
		subscriptionID = os.Getenv("K8S_SUBSCRIPTION_ID")
	}
	if subscriptionID == "" {
		return ArmConfig{}, errors.New("required env-var ARM_SUBSCRIPTION_ID is missing")
	}

	resourceGroup := os.Getenv("ARM_RESOURCE_GROUP")
	if resourceGroup == "" {
		// See #565: this is temporary code that handles the case where the app resource group and control plane group are the same
		// as it was in 0.2.
		resourceGroup = os.Getenv("K8S_RESOURCE_GROUP")
	}
	if resourceGroup == "" {
		return ArmConfig{}, errors.New("required env-var ARM_RESOURCE_GROUP is missing")
	}

	log.Printf("Using SubscriptionId = '%v' and Resource Group = '%v'", subscriptionID, resourceGroup)

	return ArmConfig{
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
		log.Println("Service Principal detected - using SP auth to get credentials")
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
		log.Println("Using Service Principal auth.")
		return auth, nil
	} else if authMethod == ManagedIdentityAuth {
		log.Println("Managed Identity detected - using Managed Identity to get credentials")

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

		log.Println("Using Managed Identity auth.")
		return auth, nil
	} else {
		log.Println("No Service Principal detected.")

		auth, err := auth.NewAuthorizerFromCLIWithResource("https://management.azure.com")

		if err != nil {
			return nil, err
		}
		log.Println("Using CLI auth.")
		return auth, nil
	}
}

// GetAuthMethod returns the authentication method used by the RP
func GetAuthMethod() string {
	clientID, ok := os.LookupEnv("AZURE_CLIENT_ID")

	if ok && clientID != "" {
		return ServicePrincipalAuth
	} else if os.Getenv("MSI_ENDPOINT") != "" || os.Getenv("IDENTITY_ENDPOINT") != "" {
		return ManagedIdentityAuth
	} else {
		return CliAuth
	}
}
