// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/golang/mock/gomock"
	armrpcv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

type ValidateInput struct {
	Name           string
	Input          []string
	ExpectedValid  bool
	ConfigHolder   framework.ConfigHolder
	ConfigureMocks func(mocks ValidateMocks)

	// CreateTempDirectory can be used to create a directory, and change directory into the
	// newly created directory before calling Validate. Set this field to the name of the directory
	// you want. The test framework will handle the cleanup.
	CreateTempDirectory string
}

type ValidateMocks struct {
	Kubernetes                  *kubernetes.MockInterface
	Namespace                   *namespace.MockInterface
	Prompter                    *prompt.MockInterface
	Helm                        *helm.MockInterface
	Setup                       *setup.MockInterface
	ApplicationManagementClient *clients.MockApplicationsManagementClient
}

func SharedCommandValidation(t *testing.T, factory func(framework framework.Factory) (*cobra.Command, framework.Runner)) {
	cmd, _ := factory(&framework.Impl{})
	require.NotNil(t, cmd.Args, "Args is required")
	require.NotEmpty(t, cmd.Example, "Example is required")
	require.NotEmpty(t, cmd.Long, "Long is required")
	require.NotEmpty(t, cmd.Short, "Short is required")
	require.NotEmpty(t, cmd.Use, "Use is required")
	require.NotNil(t, cmd.RunE, "RunE is required")
}

func SharedValidateValidation(t *testing.T, factory func(framework framework.Factory) (*cobra.Command, framework.Runner), testcases []ValidateInput) {
	t.Helper()
	for _, testcase := range testcases {
		t.Run(testcase.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			framework := &framework.Impl{
				ConfigHolder: &testcase.ConfigHolder,
				Output:       &output.MockOutput{},
			}

			if testcase.ConfigureMocks != nil {
				mocks := ValidateMocks{
					Kubernetes:                  kubernetes.NewMockInterface(ctrl),
					Namespace:                   namespace.NewMockInterface(ctrl),
					Prompter:                    prompt.NewMockInterface(ctrl),
					Helm:                        helm.NewMockInterface(ctrl),
					Setup:                       setup.NewMockInterface(ctrl),
					ApplicationManagementClient: clients.NewMockApplicationsManagementClient(ctrl),
				}

				testcase.ConfigureMocks(mocks)

				framework.KubernetesInterface = mocks.Kubernetes
				framework.NamespaceInterface = mocks.Namespace
				framework.Prompter = mocks.Prompter
				framework.HelmInterface = mocks.Helm
				framework.SetupInterface = mocks.Setup
				framework.ConnectionFactory = &connections.MockFactory{
					ApplicationsManagementClient: mocks.ApplicationManagementClient,
				}
			}

			if testcase.CreateTempDirectory != "" {
				// Will be automatically deleted after the test
				tempRoot := t.TempDir()
				combined := filepath.Join(tempRoot, testcase.CreateTempDirectory)
				err := os.MkdirAll(combined, 0775)
				require.NoError(t, err)

				wd, err := os.Getwd()
				require.NoError(t, err)
				defer func() {
					_ = os.Chdir(wd) // Restore working directory
				}()

				// Change to the new directory before running the test code.
				err = os.Chdir(combined)
				require.NoError(t, err)
			}

			cmd, runner := factory(framework)
			cmd.SetArgs(testcase.Input)
			cmd.SetContext(context.Background())

			err := cmd.ParseFlags(testcase.Input)
			require.NoError(t, err, "flag parsing failed")

			err = cmd.ValidateArgs(cmd.Flags().Args())
			if err != nil && testcase.ExpectedValid {
				require.NoError(t, err, "validation should have failed but it passed")
			} else if err != nil {
				// We expected this to fail, so it's OK if it does. No need to run Validate.
				return
			}

			err = validateRequiredFlags(cmd)
			if err != nil && testcase.ExpectedValid {
				require.NoError(t, err, "validation should have failed but it passed")
			} else if err != nil {
				// We expected this to fail, so it's OK if it does. No need to run Validate.
				return
			}

			err = runner.Validate(cmd, cmd.Flags().Args())
			if testcase.ExpectedValid {
				require.NoError(t, err, "validation should have passed but it failed")
			} else {
				require.Error(t, err, "validation should have failed but it passed")
			}
		})
	}
}

// This is really unfortunate. There's no way to have Cobra validate required flags
// without calling Run() on the command, which we don't want to do. Our workaround is to
// duplicate their logic.
func validateRequiredFlags(c *cobra.Command) error {
	flags := c.Flags()
	missingFlagNames := []string{}
	flags.VisitAll(func(f *pflag.Flag) {
		requiredAnnotation, found := f.Annotations[cobra.BashCompOneRequiredFlag]
		if !found {
			return
		}
		if (requiredAnnotation[0] == "true") && !f.Changed {
			missingFlagNames = append(missingFlagNames, f.Name)
		}
	})

	if len(missingFlagNames) > 0 {
		return fmt.Errorf(`required flag(s) "%s" not set`, strings.Join(missingFlagNames, `", "`))
	}
	return nil
}

const (
	TestWorkspaceName   = "test-workspace"
	TestEnvironmentName = "test-environment"
)

func LoadConfig(t *testing.T, yamlData string) *viper.Viper {
	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(bytes.NewBuffer([]byte(yamlData)))
	require.NoError(t, err)
	return v
}

func LoadConfigWithWorkspace(t *testing.T) *viper.Viper {

	var yamlData = `
workspaces: 
  default: test-workspace
  items: 
    test-workspace: 
      connection: 
        context: test-context
        kind: kubernetes
      environment: /planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/test-environment
      scope: /planes/radius/local/resourceGroups/test-resource-group
`

	return LoadConfig(t, yamlData)
}

func LoadConfigWithWorkspaceAndApplication(t *testing.T) *viper.Viper {

	var yamlData = `
workspaces: 
  default: test-workspace
  items: 
    test-workspace: 
      connection: 
        context: test-context
        kind: kubernetes
      defaultApplication: /planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application
      environment: /planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/test-environment
      scope: /planes/radius/local/resourceGroups/test-resource-group
`

	return LoadConfig(t, yamlData)
}

func LoadEmptyConfig(t *testing.T) *viper.Viper {

	var yamlData = `
workspaces: 
`

	return LoadConfig(t, yamlData)
}

func Create404Error() error {
	code := armrpcv1.CodeNotFound
	return &azcore.ResponseError{
		ErrorCode:  code,
		StatusCode: 404,
	}
}

func CreateResource(resourceType string, resourceName string) generated.GenericResource {
	id := fmt.Sprintf("/planes/radius/local/resourcegroups/test-environment/providers/%s/%s", resourceType, resourceName)
	location := v1.LocationGlobal

	return generated.GenericResource{
		ID:       &id,
		Name:     &resourceName,
		Type:     &resourceType,
		Location: &location,
	}
}

func CreateResourceGroup(resourceGroupName string) v20220901privatepreview.ResourceGroupResource {
	id := fmt.Sprintf("/planes/radius/local/resourcegroups/%s", resourceGroupName)
	return v20220901privatepreview.ResourceGroupResource{
		Name: &resourceGroupName,
		ID:   &id,
	}
}
