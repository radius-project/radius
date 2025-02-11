package radius

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/cmd/env/namespace"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/kubernetes/logstream"
	"github.com/radius-project/radius/pkg/cli/kubernetes/portforward"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/sdk"
)

var (
	_ provider.Provider = &radiusProvider{}
)

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &radiusProvider{
			version: version,
		}
	}
}

type radiusProvider struct {
	version string
	client  *clients.UCPApplicationsManagementClient
}

func (p *radiusProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "radius"
	resp.Version = p.version
}

func (p *radiusProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_endpoint": schema.StringAttribute{
				Required:    true,
				Description: "The API endpoint for Radius",
			},
			"api_token": schema.StringAttribute{
				Required:    true,
				Description: "The API token for Radius",
			},
		},
	}
}

func (p *radiusProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config struct {
		APIEndpoint types.String `tfsdk:"api_endpoint"`
		APIToken    types.String `tfsdk:"api_token"`
	}

	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := GetClient(ctx)
	if err != nil {
		resp.Diagnostics.AddError("failed to create client", err.Error())
		return
	}

	p.client = client
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *radiusProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *radiusProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewEnvironmentResource,
	}
}

func GetClient(ctx context.Context) (*clients.UCPApplicationsManagementClient, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configFilePath := filepath.Join(homeDir, ".rad", "config.yaml")
	v, err := cli.LoadConfig(configFilePath)
	if err != nil {
		return nil, err
	}

	// Workaround
	framework := &framework.Impl{
		Bicep:             &bicep.Impl{},
		ConnectionFactory: connections.DefaultFactory,
		ConfigHolder: &framework.ConfigHolder{
			ConfigFilePath: "$HOME/.rad/config.yaml",
			Config:         v,
		},
		Deploy:              &deploy.Impl{},
		Logstream:           &logstream.Impl{},
		Portforward:         &portforward.Impl{},
		Prompter:            &prompt.Impl{},
		ConfigFileInterface: &framework.ConfigFileInterfaceImpl{},
		KubernetesInterface: &kubernetes.Impl{},
		HelmInterface:       &helm.Impl{},
		NamespaceInterface:  &namespace.Impl{},
		// AWSClient:           aws.NewClient(),
		AzureClient: azure.NewClient(),
	}

	radiusConfig := framework.GetConfigHolder()

	section, err := cli.ReadWorkspaceSection(radiusConfig.Config)
	if err != nil {
		return nil, err
	}

	// Hardcoded workspace name
	workspace, err := section.GetWorkspace("k3d-k3s-default")
	if err != nil {
		return nil, err
	}

	connection, err := workspace.Connect(ctx)
	if err != nil {
		return nil, err
	}

	return &clients.UCPApplicationsManagementClient{
		RootScope:     workspace.Scope,
		ClientOptions: sdk.NewClientOptions(connection),
	}, nil
}
