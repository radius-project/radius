// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package config

import (
	"fmt"
	"os"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

type AzureConfig struct {
	Authorizer autorest.Authorizer
	ConfigPath string
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
		Authorizer: authorizer,
		ConfigPath: os.Getenv("RADIUS_CONFIG_PATH"),
	}, nil
}
