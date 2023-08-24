/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package common

import (
	"strings"

	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clierrors"
)

// Used in tests
const (
	AzureCredentialID = "/planes/azure/azurecloud/providers/System.Azure/credentials/%s"
	AWSCredentialID   = "/planes/aws/aws/providers/System.AWS/credentials/%s"
)

var (
	supportedProviders = []string{azure.ProviderDisplayName, aws.ProviderDisplayName}
)

// ValidateCloudProviderName checks if the given string is a supported cloud provider and returns an error if it is not.
func ValidateCloudProviderName(name string) error {
	for _, provider := range supportedProviders {
		if strings.EqualFold(name, provider) {
			return nil
		}
	}

	return clierrors.Message("The cloud provider type %q is not supported. Supported providers: %s.", name, strings.Join(supportedProviders, " "))
}
