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

// ArmConfig is the configuration we use for managing ARM resources
type ArmConfig struct {
	Auth           autorest.Authorizer
	SubscriptionID string
	ResourceGroup  string
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
	return ArmConfig{
		Auth:           *auth,
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
	}, nil
}

// GetArmAuthorizer returns an ARM authorizer for the current process
func GetArmAuthorizer() (*autorest.Authorizer, error) {
	clientID, ok := os.LookupEnv("CLIENT_ID")
	if ok && clientID != "" {
		log.Println("Service Principal detected - using SP auth to get credentials")
		clientcfg := auth.NewClientCredentialsConfig(os.Getenv("CLIENT_ID"), os.Getenv("CLIENT_SECRET"), os.Getenv("TENANT_ID"))
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
