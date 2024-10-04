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
	"fmt"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

func resolveDependencies(ctx context.Context, radius RadiusClient, scope string, environmentName string, applicationName string) (resourceGroupID string, environmentID string, applicationID string, err error) {
	found, err := findEnvironment(ctx, radius, scope, environmentName)
	if found == nil {
		return "", "", "", fmt.Errorf("could not find an environment named %q", environmentName)
	} else if err != nil {
		return "", "", "", err
	}

	environmentID = *found

	// NOTE: using resource groups with lowercase here is a workaround for a casing bug in `rad app graph`.
	// When https://github.com/radius-project/radius/issues/6422 is fixed we can use the more correct casing.
	resourceGroupID = fmt.Sprintf("/planes/radius/local/resourcegroups/%s-%s", environmentName, applicationName)
	err = createResourceGroupIfNotExists(ctx, radius, resourceGroupID)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create resource group: %w", err)
	}

	applicationID = resourceGroupID + "/providers/Applications.Core/applications/" + applicationName
	err = createApplicationIfNotExists(ctx, radius, environmentID, applicationID)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get or create application: %w", err)
	}

	return resourceGroupID, environmentID, applicationID, nil
}

func findEnvironment(ctx context.Context, radius RadiusClient, scope string, environmentName string) (*string, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("scope", scope)
	logger.Info("Listing environments.")

	response, err := radius.Environments(scope).List(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, env := range response.Value {
		if strings.EqualFold(*env.Name, environmentName) {
			return env.ID, nil
		}
	}

	return nil, nil
}

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
		Name:       to.Ptr(id.Name()),
		Properties: &ucpv20231001preview.ResourceGroupProperties{},
	}

	_, err = radius.Groups(id.RootScope()).CreateOrUpdate(ctx, id.Name(), resourceGroup, nil)
	if err != nil {
		return err
	}

	return nil
}

func createApplicationIfNotExists(ctx context.Context, radius RadiusClient, environmentID string, applicationID string) error {
	id, err := resources.Parse(applicationID)
	if err != nil {
		return err
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues("scope", id.RootScope(), "application", applicationID, "environment", environmentID)
	logger.Info("Fetching application.")

	_, err = radius.Applications(id.RootScope()).Get(context.Background(), id.Name(), nil)
	if clients.Is404Error(err) {
		// Need to create application. Keep going.
	} else if err != nil {
		return err
	} else {
		// Application already created.
		logger.Info("Application already exists.")
		return nil
	}

	app := corerpv20231001preview.ApplicationResource{
		Location: to.Ptr(v1.LocationGlobal),
		Name:     to.Ptr(id.Name()),
		Properties: &corerpv20231001preview.ApplicationProperties{
			Environment: to.Ptr(environmentID),
			Extensions: []corerpv20231001preview.ExtensionClassification{
				&corerpv20231001preview.KubernetesNamespaceExtension{
					Kind:      to.Ptr("kubernetesNamespace"),
					Namespace: to.Ptr(id.Name()),
				},
			},
		},
	}
	_, err = radius.Applications(id.RootScope()).CreateOrUpdate(ctx, id.Name(), app, nil)
	if err != nil {
		return err
	}

	return nil
}

func deleteResource(ctx context.Context, radius RadiusClient, resourceID string) (Poller[generated.GenericResourcesClientDeleteResponse], error) {
	id, err := resources.Parse(resourceID)
	if err != nil {
		return nil, err
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues("scope", id.RootScope(), "resourceType", id.Type())
	logger.Info("Deleting resource.")

	poller, err := radius.Resources(id.RootScope(), id.Type()).BeginDelete(ctx, id.Name(), nil)
	if err != nil {
		return nil, err
	}

	if !poller.Done() {
		return poller, nil
	}

	// Handle synchronous completion
	_, err = poller.Result(ctx)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func createOrUpdateResource(ctx context.Context, radius RadiusClient, resourceID string, properties map[string]any) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], error) {
	id, err := resources.Parse(resourceID)
	if err != nil {
		return nil, err
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues("scope", id.RootScope(), "resourceType", id.Type())
	logger.Info("Creating or updating resource.")

	body := generated.GenericResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Name:       to.Ptr(id.Name()),
		Properties: properties,
	}
	poller, err := radius.Resources(id.RootScope(), id.Type()).BeginCreateOrUpdate(ctx, id.Name(), body, nil)
	if err != nil {
		return nil, err
	}

	if !poller.Done() {
		return poller, nil
	}

	// Handle synchronous completion
	_, err = poller.Result(ctx)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func fetchResource(ctx context.Context, radius RadiusClient, resourceID string) (generated.GenericResourcesClientGetResponse, error) {
	id, err := resources.Parse(resourceID)
	if err != nil {
		return generated.GenericResourcesClientGetResponse{}, err
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues("scope", id.RootScope(), "resourceType", id.Type())
	logger.Info("Fetching resource.")

	return radius.Resources(id.RootScope(), id.Type()).Get(ctx, id.Name())
}

func deleteContainer(ctx context.Context, radius RadiusClient, containerID string) (Poller[corerpv20231001preview.ContainersClientDeleteResponse], error) {
	id, err := resources.Parse(containerID)
	if err != nil {
		return nil, err
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues("scope", id.RootScope(), "resourceType", id.Type())
	logger.Info("Deleting container.")

	poller, err := radius.Containers(id.RootScope()).BeginDelete(ctx, id.Name(), nil)
	if err != nil {
		return nil, err
	}

	if !poller.Done() {
		return poller, nil
	}

	// Handle synchronous completion
	_, err = poller.Result(ctx)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func createOrUpdateContainer(ctx context.Context, radius RadiusClient, containerID string, properties *corerpv20231001preview.ContainerProperties) (Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse], error) {
	id, err := resources.Parse(containerID)
	if err != nil {
		return nil, err
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues("scope", id.RootScope(), "resourceType", id.Type())
	logger.Info("Creating or updating container.")

	body := corerpv20231001preview.ContainerResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Name:       to.Ptr(id.Name()),
		Properties: properties,
	}
	poller, err := radius.Containers(id.RootScope()).BeginCreateOrUpdate(ctx, id.Name(), body, nil)
	if err != nil {
		return nil, err
	}

	if !poller.Done() {
		return poller, nil
	}

	// Handle synchronous completion
	_, err = poller.Result(ctx)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func generateDeploymentResourceName(resourceId string) string {
	resourceBaseName := strings.Split(resourceId, "/")[len(strings.Split(resourceId, "/"))-1]

	return resourceBaseName
}
