// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package connections

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/go-autorest/autorest"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/deployment"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/ucp"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/sdk"
	sdkclients "github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// DefaultFactory provides easy access to the default implementation of the factory. DO NOT modify this in your code. Even if it's for tests. DO NOT DO IT.
var DefaultFactory = &impl{}

// ConnectionFactory is a mockable abstraction for our client-server interations.
type Factory interface {
	CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error)
	CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error)
	CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error)
	CreateCloudProviderManagementClient(ctx context.Context, workspace workspaces.Workspace) (cli_credential.CredentialManagementClient, error)
}

var _ Factory = (*impl)(nil)

type impl struct {
}

func (*impl) CreateDeploymentClient(ctx context.Context, workspace workspaces.Workspace) (clients.DeploymentClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	switch c := connection.(type) {
	case *workspaces.KubernetesConnection:
		url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine(c.Overrides.UCP, c.Context)

		if err != nil {
			return nil, err
		}

		dc := azclients.NewResourceDeploymentClientWithBaseURI(url)

		// Poll faster than the default, many deployments are quick
		dc.PollingDelay = 5 * time.Second

		dc.Sender = &sender{RoundTripper: roundTripper}

		op := azclients.NewResourceDeploymentOperationsClientWithBaseURI(url)
		op.PollingDelay = 5 * time.Second
		op.Sender = &sender{RoundTripper: roundTripper}

		// This client wants a resource group name, but we store the ID instead, so compute that.
		id, err := resources.ParseScope(workspace.Scope)
		if err != nil {
			return nil, err
		}

		return &deployment.ResourceDeploymentClient{
			Client:              dc,
			OperationsClient:    op,
			RadiusResourceGroup: id.FindScope(resources.ResourceGroupsSegment),
			AzProvider:          workspace.ProviderConfig.Azure,
			AWSProvider:         workspace.ProviderConfig.AWS,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported connection type: %+v", connection)
	}

	armClientOptions := sdk.NewClientOptions(connection)
	dc, err := sdkclients.NewResourceDeploymentsClient(&sdkclients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          connection.Endpoint(),
		ARMClientOptions: armClientOptions,
	})
	if err != nil {
		return nil, err
	}

	doc, err := sdkclients.NewResourceDeploymentOperationsClient(&sdkclients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          connection.Endpoint(),
		ARMClientOptions: armClientOptions,
	})
	if err != nil {
		return nil, err
	}

	// This client wants a resource group name, but we store the ID instead, so compute that.
	id, err := resources.ParseScope(workspace.Scope)
	if err != nil {
		return nil, err
	}

	return &deployment.ResourceDeploymentClient{
		Client:              dc,
		OperationsClient:    doc,
		RadiusResourceGroup: id.FindScope(resources.ResourceGroupsSegment),
		AzProvider:          workspace.ProviderConfig.Azure,
		AWSProvider:         workspace.ProviderConfig.AWS,
	}, nil
}

func (*impl) CreateDiagnosticsClient(ctx context.Context, workspace workspaces.Workspace) (clients.DiagnosticsClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	switch c := connection.(type) {
	case *workspaces.KubernetesConnection:
		k8sClient, config, err := kubernetes.CreateTypedClient(c.Context)
		if err != nil {
			return nil, err
		}
		client, err := kubernetes.CreateRuntimeClient(c.Context, kubernetes.Scheme)
		if err != nil {
			return nil, err
		}

		baseURL, pipeline, err := kubernetes.CreateAPIServerPipeline(c.Context, c.Overrides.UCP)
		if err != nil {
			return nil, err
		}

		err = RadiusHealthCheck(ctx, workspace, pipeline, baseURL)
		if err != nil {
			return nil, err
		}

		baseURL, transporter, err := kubernetes.CreateAPIServerTransporter(c.Context, c.Overrides.UCP)
		if err != nil {
			return nil, err
		}

		clientOpts := GetClientOptions(baseURL, transporter)

		appClient, err := generated.NewGenericResourcesClient(workspace.Scope, "Applications.Core/applications", &aztoken.AnonymousCredential{}, clientOpts)
		if err != nil {
			return nil, err
		}

		cntrClient, err := generated.NewGenericResourcesClient(workspace.Scope, "Applications.Core/containers", &aztoken.AnonymousCredential{}, clientOpts)
		if err != nil {
			return nil, err
		}

		envClient, err := generated.NewGenericResourcesClient(workspace.Scope, "Applications.Core/environments", &aztoken.AnonymousCredential{}, clientOpts)
		if err != nil {
			return nil, err
		}

		gwClient, err := generated.NewGenericResourcesClient(workspace.Scope, "Applications.Core/gateways", &aztoken.AnonymousCredential{}, clientOpts)
		if err != nil {
			return nil, err
		}

		return &deployment.ARMDiagnosticsClient{
			K8sTypedClient:    k8sClient,
			RestConfig:        config,
			K8sRuntimeClient:  client,
			ApplicationClient: *appClient,
			ContainerClient:   *cntrClient,
			EnvironmentClient: *envClient,
			GatewayClient:     *gwClient,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported connection type: %+v", connection)
	}
}

func (*impl) CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	switch c := connection.(type) {
	case *workspaces.KubernetesConnection:
		baseURL, pipeline, err := kubernetes.CreateAPIServerPipeline(c.Context, c.Overrides.UCP)
		if err != nil {
			return nil, err
		}

		err = RadiusHealthCheck(ctx, workspace, pipeline, baseURL)
		if err != nil {
			return nil, err
		}

		baseURL, transporter, err := kubernetes.CreateAPIServerTransporter(c.Context, c.Overrides.UCP)
		if err != nil {
			return nil, err
		}

		return &ucp.ARMApplicationsManagementClient{
			// The client expects root scope without a leading /
			RootScope:     strings.TrimPrefix(workspace.Scope, resources.SegmentSeparator),
			ClientOptions: GetClientOptions(baseURL, transporter),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported connection type: %+v", connection)
	}
}

//nolint:all
func (*impl) CreateCloudProviderManagementClient(ctx context.Context, workspace workspaces.Workspace) (cli_credential.CredentialManagementClient, error) {
	connection, err := workspace.Connect()
	if err != nil {
		return nil, err
	}

	err = sdk.TestConnection(ctx, connection)
	if errors.Is(err, &sdk.ErrRadiusNotInstalled{}) {
		return nil, &cli.FriendlyError{Message: err.Error()}
	} else if err != nil {
		return nil, err
	}

	clientOptions := sdk.NewClientOptions(connection)

	azureCredentialClient, err := v20220901privatepreview.NewAzureCredentialClient(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, err
	}

	awsCredentialClient, err := v20220901privatepreview.NewAWSCredentialClient(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, err
	}

	return &cli_credential.UCPCredentialManagementClient{
		CredentialInterface: &cli_credential.Impl{
			AzureCredentialClient: *azureCredentialClient,
			AWSCredentialClient:   *awsCredentialClient,
		},
	}, nil
}

var _ autorest.Sender = (*sender)(nil)

type sender struct {
	RoundTripper http.RoundTripper
}

func (s *sender) Do(request *http.Request) (*http.Response, error) {
	return s.RoundTripper.RoundTrip(request)
}

// HealthCheck function checks if there is a Radius installation for the given connection.
func RadiusHealthCheck(ctx context.Context, workspace workspaces.Workspace, pipeline runtime.Pipeline, baseURL string) error {
	req, err := createHealthCheckRequest(ctx, baseURL)
	if err != nil {
		return err
	}

	resp, err := pipeline.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusNotFound {
		return &cli.FriendlyError{
			Message: fmt.Sprintf("A Radius installation could not be found for Kubernetes context %q. Use 'rad install kubernetes' to install.", workspace.Name),
		}
	}

	return nil
}

func createHealthCheckRequest(ctx context.Context, basepath string) (*policy.Request, error) {
	req, err := runtime.NewRequest(ctx, http.MethodGet, basepath)
	if err != nil {
		return nil, err
	}
	req.Raw().Header.Set("Accept", "application/json")
	return req, nil
}

// GetClientOptions function returns ClientOptions with given BaseURL and Transporter.
func GetClientOptions(baseURL string, transporter policy.Transporter) *arm.ClientOptions {
	return &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.Configuration{
				Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
					cloud.ResourceManager: {
						Endpoint: baseURL,
						Audience: "https://management.core.windows.net",
					},
				},
			},
			PerRetryPolicies: []policy.Policy{
				// Autorest will inject an empty bearer token, which conflicts with bearer auth
				// when its used by Kubernetes. We don't *ever() need Autorest to handle auth for us
				// so we just remove it.
				//
				// We'll solve this problem permanently by writing our own client.
				&RemoveAuthorizationHeaderPolicy{},
			},
			Transport: transporter,
		},
		DisableRPRegistration: true,
	}
}

var _ policy.Policy = (*RemoveAuthorizationHeaderPolicy)(nil)

type RemoveAuthorizationHeaderPolicy struct {
}

func (p *RemoveAuthorizationHeaderPolicy) Do(req *policy.Request) (*http.Response, error) {
	delete(req.Raw().Header, "Authorization")
	return req.Next()
}
