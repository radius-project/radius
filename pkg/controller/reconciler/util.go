/*
Copyright 2023.

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

package reconciler

import (
	"context"
	"os"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.yaml.in/yaml/v3"
)

func createResourceGroupIfNotExists(ctx context.Context, radius RadiusClient, resourceGroupID string) error {
	id, err := resources.Parse(resourceGroupID)
	if err != nil {
		return err
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues("scope", id.RootScope(), "resourceGroup", resourceGroupID)
	logger.Info("Fetching resourceGroup.")

	_, err = radius.Groups(id.RootScope()).Get(context.Background(), id.Name(), nil)
	if clients.Is404Error(err) {
		// Need to create resource group. Keep going.
	} else if err != nil {
		return err
	} else {
		// Resource group already created.
		logger.Info("ResourceGroup already exists.")
		return nil
	}

	resourceGroup := ucpv20231001preview.ResourceGroupResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Name:       new(id.Name()),
		Properties: &ucpv20231001preview.ResourceGroupProperties{},
	}

	_, err = radius.Groups(id.RootScope()).CreateOrUpdate(ctx, id.Name(), resourceGroup, nil)
	if err != nil {
		return err
	}

	return nil
}

func generateDeploymentResourceName(resourceId string) (string, error) {
	id, err := resources.ParseResource(resourceId)
	if err != nil {
		return "", err
	}

	return id.Name(), nil
}

func convertToARMJSONParameters(parameters map[string]string) map[string]map[string]string {
	armJSONParameters := make(map[string]map[string]string, len(parameters))
	for key, value := range parameters {
		armJSONParameters[key] = map[string]string{
			"value": value,
		}
	}
	return armJSONParameters
}

func convertFromARMJSONParameters(armJSONParameters map[string]any) map[string]string {
	parameters := make(map[string]string, len(armJSONParameters))
	for key, value := range armJSONParameters {
		if value == nil {
			continue
		}

		if valueMap, ok := value.(map[string]any); ok {
			if value, ok := valueMap["value"]; ok {
				if valueStr, ok := value.(string); ok {
					parameters[key] = valueStr
				}
			}
		}
	}
	return parameters
}

// ParseRadiusGitOpsConfig parses the Radius GitOps configuration file at the given path
// on the local filesystem.
func ParseRadiusGitOpsConfig(configFilePath string) (*RadiusGitOpsConfig, error) {
	radiusConfig := RadiusGitOpsConfig{}
	b, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(b, &radiusConfig)
	if err != nil {
		return nil, err
	}

	return &radiusConfig, nil
}
