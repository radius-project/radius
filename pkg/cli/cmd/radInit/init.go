// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radInit

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/provider/common"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	Azure EnvKind = iota
	AWS
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Installs rad with an env creation",
		Long:    "Installs rad with an env creation",
		Example: `rad env init`,
		Args:    cobra.ExactArgs(0),
		RunE:    framework.RunCommand(runner),
	}

	// Define your flags here
	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)

	return cmd, runner
}

type Runner struct {
	ConfigHolder  *framework.ConfigHolder
	Output        output.Interface
	Format        string
	Workspace     *workspaces.Workspace
	KubeContext   string
	EnvName       string
	NameSpace     string
	CloudProvider string
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args and
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return &cli.FriendlyError{Message: "workspace not mentioned"}
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return &cli.FriendlyError{Message: "output format not mentioned"}
	}
	r.Format = format
	kubeContext, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return &cli.FriendlyError{Message: "Failed to read kube config"}
	}
	fmt.Print(kubeContext.Contexts)
	//TODO: check flags if interactive or not
	r.KubeContext, err = selectKubeContext(kubeContext.CurrentContext, kubeContext.Contexts, true)
	if err != nil {
		return &cli.FriendlyError{Message: "KubeContext not mentioned"}
	}
	//TODO: check flags if interactive or not
	r.EnvName, err = common.SelectEnvironmentName(cmd, "default", true)
	if err != nil {
		return &cli.FriendlyError{Message: "Environment not mentioned"}
	}

	r.NameSpace, err = common.SelectNamespace(cmd, "default", true)
	if err != nil {
		return &cli.FriendlyError{Message: "Namespace not mentioned"}
	}

	msg := "Add cloud providers for cloud resources [y/N]?"
	cloudProvider, err := selectCloudProvider(r.Output, msg)
	if err != nil {
		return &cli.FriendlyError{Message: "Error reading cloud provider"}
	}

	provider := workspaces.AzureProvider{}
	switch cloudProvider {
	case Azure:
		msg = "Enter Azure subscription Id"
		provider.SubscriptionID, err =prompt.Text(msg, nil)
		if err != nil {
			return &cli.FriendlyError{Message: "Error reading subscription Id for azure provider"}
		}
	case AWS:
		r.Output.LogInfo("AWS is not supported")
	}

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	// Implement your command here

	return nil
}

func selectKubeContext(currentContext string, kubeContexts map[string]*api.Context, interactive bool) (string, error) {
	values := []string{}
	if interactive {
		for k, _ := range kubeContexts {
			values = append(values, k)
		}
		index, err := prompt.SelectWithDefault("Select kubeContext:", &currentContext, values)
		if err != nil {
			return "", err
		}

		return values[index], nil
	}
	return currentContext, nil
}

func selectCloudProvider(output output.Interface, selectionMessage string) (string, error) {
	yes := true
	values := []string{"Azure", "AWS", "[back]"}
	var index int
	for yes {
		yes, err := prompt.ConfirmWithDefault(selectionMessage, prompt.No)
		// yes,err := prompt.ConfirmWithDefault("Add cloud providers for cloud resources [y/N]?", prompt.No)
		if err != nil {
			return "", err
		}
		if yes {
			index, err = prompt.SelectWithDefault("", &values[0], values)
			if err != nil {
				return "", err
			}
			if values[index] == "AWS" {
				output.LogInfo("AWS not supported")
				continue
			}
			if values[index] == "[back]" {
				continue
			}
			yes = !yes
		}
	}
	return values[index], nil
}
