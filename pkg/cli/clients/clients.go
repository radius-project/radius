// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"io"
	"os"

	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/output"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
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
type DeploymentParameters = map[string]map[string]interface{}

// DeploymentOptions is the options passed when deploying an ARM-JSON template.
type DeploymentOptions struct {
	// Template is the text of the ARM-JSON template in string form.
	Template map[string]interface{}

	// Parameters is the set of parameters passed to the deployment.
	Parameters DeploymentParameters

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
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type DeploymentResult struct {
	Resources []ucpresources.ID
	Outputs   map[string]DeploymentOutput
}

// DeploymentClient is used to deploy ARM-JSON templates (compiled Bicep output).
type DeploymentClient interface {
	Deploy(ctx context.Context, options DeploymentOptions) (DeploymentResult, error)
}

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
	DeleteApplication(ctx context.Context, applicationName string) (bool, error)
	ListEnv(ctx context.Context) ([]corerp.EnvironmentResource, error)
	GetEnvDetails(ctx context.Context, envName string) (corerp.EnvironmentResource, error)
	DeleteEnv(ctx context.Context, envName string) (bool, error)
}

//go:generate mockgen -destination=./mock_cloudproviderclient.go -package=clients -self_package github.com/project-radius/radius/pkg/cli/clients github.com/project-radius/radius/pkg/cli/clients CloudProviderManagementClient

// CloudProviderManagementClient is used to interface with cloud provider configuration and credentials.
type CloudProviderManagementClient interface {
	// TODO: this interface is being added as part of v0.13 before we've nailed down completely the interactions
	// between cloud providers, UCP, and the CLI. We expect changes or possibily that this interface could be
	// replaced in the future.
	Get(ctx context.Context, name string) (CloudProviderResource, error)
	List(ctx context.Context) ([]CloudProviderResource, error)
	Put(ctx context.Context, provider AzureCloudProviderResource) error
	Delete(ctx context.Context, name string) (bool, error)
}

// CloudProviderResource is the representation of a cloud provider configuration.
type CloudProviderResource struct {
	// TODO: this is not a real resource yet. See the notes on CloudProviderManagementClient. We expect this to change significantly
	// in the future.

	// Name is the name/kind of the provider. For right now this only supports Azure.
	Name string

	// Enabled is the enabled/disabled status of the provider.
	Enabled bool
}

type AzureCloudProviderResource struct {
	CloudProviderResource

	// Credentials is used to set the credentials on Puts. It is NOT returned on Get/List.
	Credentials *ServicePrincipalCredentials
}

type ServicePrincipalCredentials struct {
	ClientID     string
	ClientSecret string
	TenantID     string
}

func ShallowCopy(params DeploymentParameters) DeploymentParameters {
	copy := DeploymentParameters{}
	for k, v := range params {
		copy[k] = v
	}

	return copy
}

type ServerLifecycleClient interface {
	GetStatus(ctx context.Context) (interface{}, []output.Column, error)
	IsRunning(ctx context.Context) (bool, error)
	EnsureStarted(ctx context.Context) error
	EnsureStopped(ctx context.Context) error
}
