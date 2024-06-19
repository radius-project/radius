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

package clients

import (
	"context"
	"io"
	"os"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	ucp_v20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucpresources "github.com/radius-project/radius/pkg/ucp/resources"
)

// NOTE: parameters in the template engine follow the structure:
//
//	{
//	  "parameter1Name": {
//	    "value": ...
//	  }
//	}
//
// Each parameter can have additional metadata besides the mandatory 'value' key.
//
// We're really only interested in 'value' and we pass the other metadata through.
//
// The full format is documented here: https://docs.microsoft.com/en-us/azure/azure-resource-manager/templates/parameter-files
//
// Note that we're only storing the 'parameters' node of the format described above.
type DeploymentParameters = map[string]map[string]any

// DeploymentOptions is the options passed when deploying an ARM-JSON template.
type DeploymentOptions struct {
	// Template is the text of the ARM-JSON template in string form.
	Template map[string]any

	// Parameters is the set of parameters passed to the deployment.
	Parameters DeploymentParameters

	// Providers are the cloud providers configured on the environment for deployment.
	Providers *Providers

	// ProgressChan is a channel used to signal progress of the deployment operation.
	// The deployment client MUST close the channel if it was provided.
	ProgressChan chan<- ResourceProgress
}

type ResourceStatus string

const (
	StatusStarted   ResourceStatus = "Started"
	StatusFailed    ResourceStatus = "Failed"
	StatusCompleted ResourceStatus = "Completed"
)

type ResourceProgress struct {
	Resource ucpresources.ID
	Status   ResourceStatus
}

type DeploymentOutput struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type DeploymentResult struct {
	Resources []ucpresources.ID
	Outputs   map[string]DeploymentOutput
}

// DeploymentClient is used to deploy ARM-JSON templates (compiled Bicep output).
type DeploymentClient interface {
	Deploy(ctx context.Context, options DeploymentOptions) (DeploymentResult, error)
}

//go:generate mockgen -typed -destination=./mock_diagnosticsclient.go -package=clients -self_package github.com/radius-project/radius/pkg/cli/clients github.com/radius-project/radius/pkg/cli/clients DiagnosticsClient

// DiagnosticsClient is used to interface with diagnostics features like logs and port-forwards.
type DiagnosticsClient interface {
	Expose(ctx context.Context, options ExposeOptions) (failed chan error, stop chan struct{}, signals chan os.Signal, err error)
	Logs(ctx context.Context, options LogsOptions) ([]LogStream, error)
	GetPublicEndpoint(ctx context.Context, options EndpointOptions) (*string, error)
}

type ApplicationStatus struct {
	Name          string
	ResourceCount int
	Gateways      []GatewayStatus
}

type GatewayStatus struct {
	Name     string
	Endpoint string
}

type EndpointOptions struct {
	ResourceID ucpresources.ID
}

type ExposeOptions struct {
	Application string
	Resource    string
	Port        int
	RemotePort  int
	Replica     string
}

type LogsOptions struct {
	Application string
	Resource    string
	Follow      bool
	Container   string
	Replica     string
}

type LogStream struct {
	Name   string
	Stream io.ReadCloser
}

//go:generate mockgen -typed -destination=./mock_applicationsclient.go -package=clients -self_package github.com/radius-project/radius/pkg/cli/clients github.com/radius-project/radius/pkg/cli/clients ApplicationsManagementClient

// ApplicationsManagementClient is used to interface with management features like listing resources by app, show details of a resource.
type ApplicationsManagementClient interface {
	// ListResourcesOfType lists all resources of a given type in the configured scope.
	ListResourcesOfType(ctx context.Context, resourceType string) ([]generated.GenericResource, error)

	// ListResourcesOfTypeInApplication lists all resources of a given type in a given application in the configured scope.
	ListResourcesOfTypeInApplication(ctx context.Context, applicationNameOrID string, resourceType string) ([]generated.GenericResource, error)

	// ListResourcesOfTypeInEnvironment lists all resources of a given type in a given environment in the configured scope.
	ListResourcesOfTypeInEnvironment(ctx context.Context, environmentNameOrID string, resourceType string) ([]generated.GenericResource, error)

	// ListResourcesInApplication lists all resources in a given application in the configured scope.
	ListResourcesInApplication(ctx context.Context, applicationNameOrID string) ([]generated.GenericResource, error)

	// ListResourcesInEnvironment lists all resources in a given environment in the configured scope.
	ListResourcesInEnvironment(ctx context.Context, environmentNameOrID string) ([]generated.GenericResource, error)

	// GetResource retrieves a resource by its type and name (or id).
	GetResource(ctx context.Context, resourceType string, resourceNameOrID string) (generated.GenericResource, error)

	// CreateOrUpdateResource creates or updates a resource using its type name (or id).
	CreateOrUpdateResource(ctx context.Context, resourceType string, resourceNameOrID string, resource *generated.GenericResource) (generated.GenericResource, error)

	// DeleteResource deletes a resource by its type and name (or id).
	DeleteResource(ctx context.Context, resourceType string, resourceNameOrID string) (bool, error)

	// ListApplications lists all applications in the configured scope.
	ListApplications(ctx context.Context) ([]corerp.ApplicationResource, error)

	// GetApplication retrieves an application by its name (or id).
	GetApplication(ctx context.Context, applicationNameOrID string) (corerp.ApplicationResource, error)

	// GetApplicationGraph retrieves the application graph of an application by its name (or id).
	GetApplicationGraph(ctx context.Context, applicationNameOrID string) (corerp.ApplicationGraphResponse, error)

	// CreateOrUpdateApplication creates or updates an application by its name (or id).
	CreateOrUpdateApplication(ctx context.Context, applicationNameOrID string, resource *corerp.ApplicationResource) error

	// CreateApplicationIfNotFound creates an application if it does not exist.
	CreateApplicationIfNotFound(ctx context.Context, applicationNameOrID string, resource *corerp.ApplicationResource) error

	// DeleteApplication deletes an application and all of its resources by its name (or id).
	DeleteApplication(ctx context.Context, applicationNameOrID string) (bool, error)

	// ListEnvironments lists all environments in the configured scope (assumes configured scope is a resource group).
	ListEnvironments(ctx context.Context) ([]corerp.EnvironmentResource, error)

	// ListEnvironmentsAll lists all environments across resource groups.
	ListEnvironmentsAll(ctx context.Context) ([]corerp.EnvironmentResource, error)

	// GetEnvironment retrieves an environment by its name (in the configured scope) or resource ID.
	GetEnvironment(ctx context.Context, environmentNameOrID string) (corerp.EnvironmentResource, error)

	// GetRecipeMetadata shows recipe details including list of all parameters for a given recipe registered to an environment.
	GetRecipeMetadata(ctx context.Context, environmentNameOrID string, recipe corerp.RecipeGetMetadata) (corerp.RecipeGetMetadataResponse, error)

	// CreateOrUpdateEnvironment creates an environment by its name (or id).
	CreateOrUpdateEnvironment(ctx context.Context, environmentNameOrID string, resource *corerp.EnvironmentResource) error

	// DeleteEnvironment deletes an environment and all of its resources by its name (in the configured scope) or resource ID.
	DeleteEnvironment(ctx context.Context, environmentNameOrID string) (bool, error)

	// ListResourceGroups lists all resource groups in the configured scope.
	ListResourceGroups(ctx context.Context, planeName string) ([]ucp_v20231001preview.ResourceGroupResource, error)

	// GetResourceGroup retrieves a resource group by its name.
	GetResourceGroup(ctx context.Context, planeName string, resourceGroupName string) (ucp_v20231001preview.ResourceGroupResource, error)

	// CreateOrUpdateResourceGroup creates a resource group by its name.
	CreateOrUpdateResourceGroup(ctx context.Context, planeName string, resourceGroupName string, resource *ucp_v20231001preview.ResourceGroupResource) error

	// DeleteResourceGroup deletes a resource group by its name.
	DeleteResourceGroup(ctx context.Context, planeName string, resourceGroupName string) (bool, error)

	// ListResourceProviders lists all resource providers in the configured scope.
	ListResourceProviders(ctx context.Context, planeName string) ([]ucp_v20231001preview.ResourceProviderResource, error)

	// GetResourceProvider gets the resource provider with the specified name in the configured scope.
	GetResourceProvider(ctx context.Context, planeName string, providerNamespace string) (ucp_v20231001preview.ResourceProviderResource, error)

	// CreateOrUpdateResourceProvider creates or updates a resource provider in the configured scope.
	CreateOrUpdateResourceProvider(ctx context.Context, planeName string, providerNamespace string, resource *ucp_v20231001preview.ResourceProviderResource) (ucp_v20231001preview.ResourceProviderResource, error)

	// DeleteResourceProvider deletes a resource provider in the configured scope.
	DeleteResourceProvider(ctx context.Context, planeName string, providerNamespace string) (bool, error)
}

// ShallowCopy creates a shallow copy of the DeploymentParameters object by iterating through the original object and
// copying each key-value pair into a new object.
func ShallowCopy(params DeploymentParameters) DeploymentParameters {
	copy := DeploymentParameters{}
	for k, v := range params {
		copy[k] = v
	}

	return copy
}
