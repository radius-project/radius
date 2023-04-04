// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package common

import (
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
)

// Used in tests
const (
	AzureCloudProvider = "Azure"
	AWSCloudProvider   = "AWS"
	AzureCredentialID  = "/planes/azure/azurecloud/providers/System.Azure/credentials/%s"
	AWSCredentialID    = "/planes/aws/aws/providers/System.AWS/credentials/%s"
)

var (
	supportedProviders = []string{AzureCloudProvider, AWSCloudProvider}
)

func ValidateCloudProviderName(name string) error {
	for _, provider := range supportedProviders {
		if strings.EqualFold(name, provider) {
			return nil
		}
	}

	return &cli.FriendlyError{Message: fmt.Sprintf("Cloud provider type %q is not supported. ", strings.Join(supportedProviders, " "))}
}
