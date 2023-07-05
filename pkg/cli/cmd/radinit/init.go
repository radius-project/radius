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

package radinit

import (
	"context"
	"os"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"

	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/cmd"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/spf13/cobra"
)

// NOTE: this command is very super big so it's broken up amongst a few files.

// NewCommand creates an instance of the command and runner for the `rad init` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Initialize Radius",
		Long:    "Interactively initialize the Radius control-plane, create an environment, and configure a workspace",
		Example: `rad init`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	// Define your flags here
	commonflags.AddOutputFlag(cmd)
	cmd.Flags().Bool("dev", false, "Setup Radius for development")
	return cmd, runner
}

// Runner is the runner implementation for the `rad init` command.
type Runner struct {
	azureClient azure.Client
	awsClient   aws.Client

	// ConfigFileInterface is the interface for the config file.
	ConfigFileInterface framework.ConfigFileInterface

	// ConfigHolder is the interface for the config holder.
	ConfigHolder *framework.ConfigHolder

	// ConnectionFactory is the interface for the connection factory.
	ConnectionFactory connections.Factory

	// HelmInterface is the interface for the helm client.
	HelmInterface helm.Interface

	// KubernetesInterface is the interface for the kubernetes client.
	KubernetesInterface kubernetes.Interface

	// Output is the interface for console output.
	Output output.Interface

	// Prompter is the interface for the prompter.
	Prompter prompt.Interface

	// DevRecipeClient is the interface for the dev recipe client.
	DevRecipeClient DevRecipeClient

	// Format is the output format.
	Format string

	// Workspace is the workspace to use. This will be populated by Validate.
	Workspace *workspaces.Workspace

	// Dev determines whether or not we're in dev mode.
	Dev bool

	// Options provides the options to used for Radius initialization. This will be populated by Validate.
	Options *initOptions
}

// NewRunner creates a new instance of the `rad init` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		ConnectionFactory:   factory.GetConnectionFactory(),
		Prompter:            factory.GetPrompter(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		KubernetesInterface: factory.GetKubernetesInterface(),
		HelmInterface:       factory.GetHelmInterface(),
		DevRecipeClient:     NewDevRecipeClient(),
		awsClient:           factory.GetAWSClient(),
		azureClient:         factory.GetAzureClient(),
	}
}

// Validate runs validation for the `rad init` command.
//
// Validates the user prompts, values provided and builds the picture for the backend to execute
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	r.Dev, err = cmd.Flags().GetBool("dev")
	if err != nil {
		return err
	}

	for {
		options, workspace, err := r.enterInitOptions(cmd.Context())
		if err != nil {
			return err
		}

		// Show a confirmation screen unless we're in dev mode.
		confirmed := true
		if !r.Dev {
			confirmed, err = r.confirmOptions(cmd.Context(), options)
			if err != nil {
				return err
			}
		}

		if confirmed {
			r.Options = options
			r.Workspace = workspace
			return nil
		}

		// User did not confirm the summary, so gather input again.
	}
}

// Run runs the `rad init` command.
//
// Creates radius resources, azure resources if required based on the user input, command flags
func (r *Runner) Run(ctx context.Context) error {
	config := r.ConfigFileInterface.ConfigFromContext(ctx)

	// Use this channel to send progress updates to the UI.
	progressChan := make(chan progressMsg)
	progressCompleteChan := make(chan error)
	progress := progressMsg{}

	go func() {
		// Show dynamic UI.
		err := r.showProgress(ctx, r.Options, progressChan)
		if err != nil {
			progressCompleteChan <- err
		}
		close(progressCompleteChan)
	}()

	if r.Options.Cluster.Install {
		// Install radius control plane
		err := installRadius(ctx, r)
		if err != nil {
			return clierrors.MessageWithCause(err, "Failed to install Radius.")
		}
	}
	progress.InstallComplete = true
	progressChan <- progress

	if r.Options.Environment.Create {
		client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
		if err != nil {
			return err
		}

		//ignore the id of the resource group created
		err = client.CreateUCPGroup(ctx, "radius", "local", r.Options.Environment.Name, ucp.ResourceGroupResource{
			Location: to.Ptr(v1.LocationGlobal),
		})
		if err != nil {
			return clierrors.MessageWithCause(err, "Failed to create Azure resource group.")
		}

		// TODO: we TEMPORARILY create a resource group in the deployments plane because the deployments RP requires it.
		// We'll remove this in the future.
		err = client.CreateUCPGroup(ctx, "deployments", "local", r.Options.Environment.Name, ucp.ResourceGroupResource{
			Location: to.Ptr(v1.LocationGlobal),
		})
		if err != nil {
			return err
		}

		providerList := []any{}
		if r.Options.CloudProviders.Azure != nil {
			providerList = append(providerList, r.Options.CloudProviders.Azure)
		}
		if r.Options.CloudProviders.AWS != nil {
			providerList = append(providerList, r.Options.CloudProviders.AWS)
		}

		providers, err := cmd.CreateEnvProviders(providerList)
		if err != nil {
			return err
		}

		var recipes map[string]map[string]corerp.EnvironmentRecipePropertiesClassification
		if r.Options.Recipes.DevRecipes {
			recipes, err = r.DevRecipeClient.GetDevRecipes(ctx)
			if err != nil {
				return err
			}
		}

		envProperties := corerp.EnvironmentProperties{
			Compute: &corerp.KubernetesCompute{
				Namespace: to.Ptr(r.Options.Environment.Namespace),
			},
			Providers: &providers,
			Recipes:   recipes,
		}

		err = client.CreateEnvironment(ctx, r.Options.Environment.Name, v1.LocationGlobal, &envProperties)
		if err != nil {
			return clierrors.MessageWithCause(err, "Failed to create environment.")
		}

		credentialClient, err := r.ConnectionFactory.CreateCredentialManagementClient(ctx, *r.Workspace)
		if err != nil {
			return err
		}
		if r.Options.CloudProviders.Azure != nil {
			credential := r.getAzureCredential()
			err := credentialClient.PutAzure(ctx, credential)
			if err != nil {
				return clierrors.MessageWithCause(err, "Failed to configure Azure credentials.")
			}
		}
		if r.Options.CloudProviders.AWS != nil {
			credential := r.getAWSCredential()
			err := credentialClient.PutAWS(ctx, credential)
			if err != nil {
				return clierrors.MessageWithCause(err, "Failed to configure AWS credentials.")
			}
		}
	}
	progress.EnvironmentComplete = true
	progressChan <- progress

	if r.Options.Application.Scaffold {
		client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
		if err != nil {
			return err
		}

		// Initialize the application resource if it's not found. This supports the scenario where the application
		// resource is not defined in bicep.
		err = client.CreateApplicationIfNotFound(ctx, r.Options.Application.Name, corerp.ApplicationResource{
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &corerp.ApplicationProperties{
				Environment: &r.Workspace.Environment,
			},
		})
		if err != nil {
			return err
		}

		// Scaffold application files in the current directory
		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		err = setup.ScaffoldApplication(wd, r.Options.Application.Name)
		if err != nil {
			return err
		}
	}
	progress.ApplicationComplete = true
	progressChan <- progress

	err := r.ConfigFileInterface.EditWorkspaces(ctx, config, r.Workspace)
	if err != nil {
		return err
	}
	progress.ConfigComplete = true
	progressChan <- progress

	// Wait for UI to complete.
	err = <-progressCompleteChan
	if err != nil {
		return err
	}

	return nil
}

func (r *Runner) getAzureCredential() ucp.AzureCredentialResource {
	return ucp.AzureCredentialResource{
		Location: to.Ptr(v1.LocationGlobal),
		Type:     to.Ptr(cli_credential.AzureCredential),
		Properties: &ucp.AzureServicePrincipalProperties{
			Storage: &ucp.CredentialStorageProperties{
				Kind: to.Ptr(string(ucp.CredentialStorageKindInternal)),
			},
			TenantID:     &r.Options.CloudProviders.Azure.ServicePrincipal.TenantID,
			ClientID:     &r.Options.CloudProviders.Azure.ServicePrincipal.ClientID,
			ClientSecret: &r.Options.CloudProviders.Azure.ServicePrincipal.ClientSecret,
		},
	}
}

func (r *Runner) getAWSCredential() ucp.AWSCredentialResource {
	return ucp.AWSCredentialResource{
		Location: to.Ptr(v1.LocationGlobal),
		Type:     to.Ptr(cli_credential.AWSCredential),
		Properties: &ucp.AWSAccessKeyCredentialProperties{
			Storage: &ucp.CredentialStorageProperties{
				Kind: to.Ptr(string(ucp.CredentialStorageKindInternal)),
			},
			AccessKeyID:     &r.Options.CloudProviders.AWS.AccessKeyID,
			SecretAccessKey: &r.Options.CloudProviders.AWS.SecretAccessKey,
		},
	}
}

func installRadius(ctx context.Context, r *Runner) error {
	cliOptions := helm.CLIClusterOptions{
		Radius: helm.RadiusOptions{},
	}

	clusterOptions := helm.PopulateDefaultClusterOptions(cliOptions)

	// Ignore existing radius installation because we already asked the user whether to re-install or not
	_, err := r.HelmInterface.InstallRadius(ctx, clusterOptions, r.Options.Cluster.Context)
	if err != nil {
		return err
	}

	return nil
}
