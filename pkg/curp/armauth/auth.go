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
	ClientID       string
}

// GetArmConfig gets the configuration we use for managing ARM resources
func GetArmConfig() (ArmConfig, error) {
	auth, clientID, err := GetArmAuthorizerAndClientID()
	if err != nil {
		return ArmConfig{}, err
	}
	log.Printf("@@@ using client id: %v", clientID)

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
		ClientID:       clientID,
	}, nil
}

// GetArmAuthorizerAndClientID returns an ARM authorizer and the client ID for the current process
func GetArmAuthorizerAndClientID() (*autorest.Authorizer, string, error) {
	clientID, ok := os.LookupEnv("CLIENT_ID")

	if ok && clientID != "" {
		log.Println("Service Principal detected - using SP auth to get credentials")
		clientcfg := auth.NewClientCredentialsConfig(os.Getenv("CLIENT_ID"), os.Getenv("CLIENT_SECRET"), os.Getenv("TENANT_ID"))
		auth, err := clientcfg.Authorizer()
		if err != nil {
			return nil, "", err
		}

		token, err := clientcfg.ServicePrincipalToken()
		if err != nil {
			return nil, "", err
		}

		err = token.EnsureFresh()
		if err != nil {
			return nil, "", err
		}
		log.Println("Using Service Principal auth.")
		return &auth, clientcfg.ClientID, nil
	} else if os.Getenv("MSI_ENDPOINT") != "" || os.Getenv("IDENTITY_ENDPOINT") != "" {
		log.Println("Managed Identity detected - using Managed Identity to get credentials")

		env, _ := auth.GetSettingsFromEnvironment()
		msiconfig := env.GetMSI()
		log.Printf("@@@ msiconfig clientid: %s, resource: %s", msiconfig.ClientID, msiconfig.Resource)

		config := auth.NewMSIConfig()
		token, err := config.ServicePrincipalToken()
		if err != nil {
			return nil, "", err
		}

		err = token.EnsureFresh()
		if err != nil {
			return nil, "", err
		}

		auth, err := config.Authorizer()
		if err != nil {
			return nil, "", err
		}

		log.Printf("Using Managed Identity auth. Client ID: %s", config.ClientID)
		return &auth, config.ClientID, nil
	} else {
		log.Println("No Service Principal detected.")

		auth, err := auth.NewAuthorizerFromCLIWithResource("https://management.azure.com")

		// cli.Profile
		// var token *cli.Token
		// token, err = cli.GetTokenFromCLI("https://management.azure.com")
		// fmt.Println(token.ClientID)

		// // u := graphrbac.NewSignedInUserClient("72f988bf-86f1-41af-91ab-2d7cd011db47")
		// // u.Authorizer = auth
		// // user, err := u.Get(context.TODO())
		// // fmt.Printf("@@@ current user: %v", user)

		// // ac := graphrbac.NewApplicationsClient("72f988bf-86f1-41af-91ab-2d7cd011db47")
		// // ac.Authorizer = auth
		// // list, err := ac.List(context.TODO(), "")
		// // fmt.Println(list)

		if err != nil {
			return nil, "", err
		}
		log.Println("Using CLI auth.")
		return &auth, "", nil
	}
}
