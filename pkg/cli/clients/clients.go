// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"io"
	"os"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/radclient"
)

// NOTE: parameters in the template engine follow the structure:
//
// {
//   "parameter1Name": {
//     "value": ...
//   }
// }
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
	Template string

	// Parameters is the set of parameters passed to the deployment.
	Parameters DeploymentParameters
}

type DeploymentOutput struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type DeploymentResult struct {
	Resources []azresources.ResourceID
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

type EndpointOptions struct {
	ResourceID azresources.ResourceID
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

// ManagementClient is used to interface with management features like listing applications and resources.
type ManagementClient interface {
	ListApplications(ctx context.Context) (*radclient.ApplicationList, error)
	ShowApplication(ctx context.Context, applicationName string) (*radclient.ApplicationResource, error)
	DeleteApplication(ctx context.Context, applicationName string) error

	ShowResource(ctx context.Context, applicationName string, resourceType string, resourceName string) (interface{}, error)
	ListAllResourcesByApplication(ctx context.Context, applicationName string) (*radclient.RadiusResourceList, error)
}

func ShallowCopy(params DeploymentParameters) DeploymentParameters {
	copy := DeploymentParameters{}
	for k, v := range params {
		copy[k] = v
	}

	return copy
}
