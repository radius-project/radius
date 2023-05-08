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

	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	ucp_v20220901privatepreview "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	ucpresources "github.com/project-radius/radius/pkg/ucp/resources"
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

//go:generate mockgen -destination=./mock_diagnosticsclient.go -package=clients -self_package github.com/project-radius/radius/pkg/cli/clients github.com/project-radius/radius/pkg/cli/clients DiagnosticsClient

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

//go:generate mockgen -destination=./mock_applicationsclient.go -package=clients -self_package github.com/project-radius/radius/pkg/cli/clients github.com/project-radius/radius/pkg/cli/clients ApplicationsManagementClient

// ApplicationsManagementClient is used to interface with management features like listing resources by app, show details of a resource.
type ApplicationsManagementClient interface {
	ListAllResourcesByType(ctx context.Context, resourceType string) ([]generated.GenericResource, error)
	ListAllResourcesOfTypeInApplication(ctx context.Context, applicationName string, resourceType string) ([]generated.GenericResource, error)
	ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]generated.GenericResource, error)
	ListAllResourcesOfTypeInEnvironment(ctx context.Context, environmentName string, resourceType string) ([]generated.GenericResource, error)
	ListAllResourcesByEnvironment(ctx context.Context, environmentName string) ([]generated.GenericResource, error)
	ShowResource(ctx context.Context, resourceType string, resourceName string) (generated.GenericResource, error)
	DeleteResource(ctx context.Context, resourceType string, resourceName string) (bool, error)
	ListApplications(ctx context.Context) ([]corerp.ApplicationResource, error)
	ShowApplication(ctx context.Context, applicationName string) (corerp.ApplicationResource, error)

	// CreateOrUpdateApplication creates or updates an application.
	CreateOrUpdateApplication(ctx context.Context, applicationName string, resource corerp.ApplicationResource) error

	// CreateApplicationIfNotFound creates an application if it does not exist.
	CreateApplicationIfNotFound(ctx context.Context, applicationName string, resource corerp.ApplicationResource) error

	DeleteApplication(ctx context.Context, applicationName string) (bool, error)
	CreateEnvironment(ctx context.Context, envName string, location string, envProperties *corerp.EnvironmentProperties) (bool, error)

	// ListEnvironmentsInResourceGroup lists all environments in the configured scope (assumes configured scope is a resource group)
	ListEnvironmentsInResourceGroup(ctx context.Context) ([]corerp.EnvironmentResource, error)

	// ListEnvironmentsAll lists all environments across resource groups.
	ListEnvironmentsAll(ctx context.Context) ([]corerp.EnvironmentResource, error)
	GetEnvDetails(ctx context.Context, envName string) (corerp.EnvironmentResource, error)
	DeleteEnv(ctx context.Context, envName string) (bool, error)
	CreateUCPGroup(ctx context.Context, planeType string, planeName string, resourceGroupName string, resourceGroup ucp_v20220901privatepreview.ResourceGroupResource) (bool, error)
	DeleteUCPGroup(ctx context.Context, planeType string, planeName string, resourceGroupName string) (bool, error)
	ShowUCPGroup(ctx context.Context, planeType string, planeName string, resourceGroupName string) (ucp_v20220901privatepreview.ResourceGroupResource, error)
	ListUCPGroup(ctx context.Context, planeType string, planeName string) ([]ucp_v20220901privatepreview.ResourceGroupResource, error)

	// ShowRecipe shows recipe details including list of all parameters for a given recipe registered to an environment
	ShowRecipe(ctx context.Context, environmentName string, recipe corerp.Recipe) (corerp.EnvironmentRecipeProperties, error)
}

func ShallowCopy(params DeploymentParameters) DeploymentParameters {
	copy := DeploymentParameters{}
	for k, v := range params {
		copy[k] = v
	}

	return copy
}
