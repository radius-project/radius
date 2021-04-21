// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package config

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/marstr/randname"
)

type AzureConfig struct {
	Authorizer      autorest.Authorizer
	ConfigPath      string
	subscriptionID  string
	locationDefault string
	cloudName       string
	baseGroupName   string
}

func NewAzureConfig() (*AzureConfig, error) {
	// This will read the standard set of Azure env-vars and fall-back to CLI auth if they are not present.
	var authorizer autorest.Authorizer
	var err error
	if os.Getenv("AZURE_CLIENT_ID") != "" || os.Getenv("AZURE_CLIENT_SECRET") != "" || os.Getenv("AZURE_TENANT_ID") != "" {
		authorizer, err = auth.NewAuthorizerFromEnvironment()
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate with Service Principal auth: %w", err)
		}
	} else {
		authorizer, err = auth.NewAuthorizerFromCLI()
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate with CLI auth: %w", err)
		}
	}

	return &AzureConfig{
		Authorizer:      authorizer,
		ConfigPath:      os.Getenv("RADIUS_CONFIG_PATH"),
		subscriptionID:  os.Getenv("INTEGRATION_TEST_SUBSCRIPTION_ID"),
		locationDefault: os.Getenv("INTEGRATION_TEST_DEFAULT_LOCATION"),
		baseGroupName:   os.Getenv("INTEGRATION_TEST_BASE_GROUP_NAME"),
		cloudName:       "AzurePublicCloud",
	}, nil
}

// GenerateGroupName generates a resource group name with INTEGRATION_TEST_BASE_GROUP_NAME as the prefix
// and adds a 5-character random string to it.
func (config *AzureConfig) GenerateGroupName(affixes ...string) string {
	b := bytes.NewBufferString(config.baseGroupName)
	b.WriteRune('-')
	for _, affix := range affixes {
		b.WriteString(affix)
		b.WriteRune('-')
	}
	return randname.GenerateWithPrefix(b.String(), 5)
}

// SubscriptionID returns the subscription ID
func (config *AzureConfig) SubscriptionID() string {
	return config.subscriptionID
}

// DefaultLocation returns the location default
func (config *AzureConfig) DefaultLocation() string {
	return config.locationDefault
}
