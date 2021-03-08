// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package config

import (
	"bytes"
	"os"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2019-05-01/resources"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/marstr/randname"
)

var (
	// AzureConfig contains the configuration info to authorize with ARM
	AzureConfig *azureConfig
)

type azureConfig struct {
	clientID        string
	clientSecret    string
	tenantID        string
	subscriptionID  string
	locationDefault string
	cloudName       string
	baseGroupName   string
	environment     *azure.Environment
}

func init() {
	AzureConfig.initialize()
}

// Read test configuration from environment variables
func (config *azureConfig) initialize() {
	config.clientID = os.Getenv("AZURE_CLIENT_ID")
	config.clientSecret = os.Getenv("AZURE_CLIENT_SECRET")
	config.tenantID = os.Getenv("AZURE_TENANT_ID")
	config.subscriptionID = os.Getenv("INTEGRATION_TEST_SUBSCRIPTION_ID")
	config.locationDefault = os.Getenv("INTEGRATION_TEST_DEFAULT_LOCATION")
	config.baseGroupName = os.Getenv("INTEGRATION_TEST_BASE_GROUP_NAME")
	config.cloudName = "AzurePublicCloud"
}

// GenerateGroupName generates a resource group name with INTEGRATION_TEST_BASE_GROUP_NAME as the prefix
// and adds a 5-character random string to it.
func (config *azureConfig) GenerateGroupName(affixes ...string) string {
	b := bytes.NewBufferString(config.baseGroupName)
	b.WriteRune('-')
	for _, affix := range affixes {
		b.WriteString(affix)
		b.WriteRune('-')
	}
	return randname.GenerateWithPrefix(b.String(), 5)
}

// SubscriptionID returns the subscription ID
func (config *azureConfig) SubscriptionID() string {
	return config.subscriptionID
}

// ClientID returns the client ID
func (config *azureConfig) ClientID() string {
	return config.clientID
}

// DefaultLocation returns the location default
func (config *azureConfig) DefaultLocation() string {
	return config.locationDefault
}

// GetResourcesClient initializes and returns a resources.Client
func (config *azureConfig) GetResourcesClient() (resources.Client, error) {
	resourcesClient := resources.NewClient(config.subscriptionID)
	// a, _ := iam.GetResourceManagementAuthorizer()
	a, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		return resources.Client{}, err
	}
	resourcesClient.Authorizer = a
	return resourcesClient, nil
}

// GetGroupsClient initializes and returns a resources.GroupsClient
func (config *azureConfig) GetGroupsClient() (resources.GroupsClient, error) {
	groupsClient := resources.NewGroupsClient(AzureConfig.subscriptionID)
	a, err := auth.NewAuthorizerFromEnvironment()
	if err != nil {
		return resources.GroupsClient{}, err
	}
	groupsClient.Authorizer = a
	return groupsClient, nil
}
