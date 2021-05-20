// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armauth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/Azure/azure-sdk-for-go/services/msi/mgmt/2018-11-30/msi"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

const (
	ServicePrincipalAuth = "ServicePrincipal"
	ManagedIdentityAuth  = "ManagedIdentity"
	CliAuth              = "CLI"
)

// ArmConfig is the configuration we use for managing ARM resources
type ArmConfig struct {
	Auth           autorest.Authorizer
	SubscriptionID string
	ResourceGroup  string
	ClientID       string
}

// GetArmConfig gets the configuration we use for managing ARM resources
func GetArmConfig() (ArmConfig, error) {
	auth, err := GetArmAuthorizer()
	if err != nil {
		return ArmConfig{}, err
	}

	subscriptionID := os.Getenv("ARM_SUBSCRIPTION_ID")
	if subscriptionID == "" {
		subscriptionID = os.Getenv("K8S_SUBSCRIPTION_ID")
	}

	if subscriptionID == "" {
		return ArmConfig{}, errors.New("required env-var ARM_SUBSCRIPTION_ID or K8S_SUBSCRIPTION_ID is missing")
	}

	resourceGroup := os.Getenv("ARM_RESOURCE_GROUP")
	if resourceGroup == "" {
		resourceGroup = os.Getenv("K8S_RESOURCE_GROUP")
	}

	if resourceGroup == "" {
		return ArmConfig{}, errors.New("required env-var ARM_RESOURCE_GROUP or K8S_RESOURCE_GROUP is missing")
	}

	log.Printf("Using SubscriptionId = '%v' and Resource Group = '%v'", subscriptionID, resourceGroup)

	clientID, err := GetClientIDForRP(subscriptionID, resourceGroup, *auth)
	if err != nil || clientID == "" {
		return ArmConfig{}, fmt.Errorf("unable to get clientID to use for role assignments: %w", err)
	}

	return ArmConfig{
		Auth:           *auth,
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
		ClientID:       clientID,
	}, nil
}

// GetArmAuthorizer returns an ARM authorizer and the client ID for the current process
func GetArmAuthorizer() (*autorest.Authorizer, error) {
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
		return &auth, nil
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
		return &auth, nil
	} else {
		log.Println("No Service Principal detected.")

		auth, err := auth.NewAuthorizerFromCLIWithResource("https://management.azure.com")

		if err != nil {
			return nil, err
		}
		log.Println("Using CLI auth.")
		return &auth, nil
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

// GetClientIDForRP gets the Identity for the RP.
// This will be either a serviceprincipal clientID, SystemAssigned Identity or ObjectID for the CLI user based on the auth mechanism
func GetClientIDForRP(subscriptionID, resourceGroup string, auth autorest.Authorizer) (string, error) {
	authMethod := GetAuthMethod()
	if authMethod == ServicePrincipalAuth {
		return os.Getenv("AZURE_CLIENT_ID"), nil
	} else if authMethod == ManagedIdentityAuth {
		log.Println("Managed Identity detected - using Managed Identity to get credentials")

		rpName, ok := os.LookupEnv("RP_NAME")
		if !ok {
			log.Fatalln("Could not read RadiusRP name")
		}

		rp := azure.Resource{
			SubscriptionID: subscriptionID,
			ResourceGroup:  resourceGroup,
			Provider:       "Microsoft.Web",
			ResourceType:   "sites",
			ResourceName:   rpName,
		}
		mc := msi.NewSystemAssignedIdentitiesClient(subscriptionID)
		mc.Authorizer = auth
		si, err := mc.GetByScope(context.TODO(), rp.String())

		if err != nil {
			return "", fmt.Errorf("Unable to get system assigned identity over scope: %v: %w", rp.String(), err)
		}

		return si.PrincipalID.String(), nil
	} else {
		rpClientID, ok := os.LookupEnv("AZURE_USER_OBJECT_ID")
		if !ok {
			return "", errors.New("Unable to get AZURE_USER_OBJECT_ID environment variable. Please set this to the output of 'az ad signed-in-user show --query objectId --output tsv'")
		}
		return rpClientID, nil
	}
}
