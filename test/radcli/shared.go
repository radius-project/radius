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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
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

	// ValidateCallback can be used to support a validation callback that will run after all other validation.
	//
	// This can be used to validate side-effects that occur in a Runner's Validate() function.
	ValidateCallback func(t *testing.T, runner framework.Runner)

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
	ApplicationManagementClient *clients.MockApplicationsManagementClient
	AzureClient                 *azure.MockClient
	AWSClient                   *aws.MockClient
}

// # Function Explanation
//
// SharedCommandValidation tests that the command created by the factory function has all the required fields set.
func SharedCommandValidation(t *testing.T, factory func(framework framework.Factory) (*cobra.Command, framework.Runner)) {
	cmd, _ := factory(&framework.Impl{})
	require.NotNil(t, cmd.Args, "Args is required")
	require.NotEmpty(t, cmd.Example, "Example is required")
	require.NotEmpty(t, cmd.Long, "Long is required")
	require.NotEmpty(t, cmd.Short, "Short is required")
	require.NotEmpty(t, cmd.Use, "Use is required")
	require.NotNil(t, cmd.RunE, "RunE is required")
}

// # Function Explanation
//
// SharedValidateValidation runs a series of tests to validate command line arguments and flags, and returns
// an error if validation fails.
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
					ApplicationManagementClient: clients.NewMockApplicationsManagementClient(ctrl),
					AzureClient:                 azure.NewMockClient(ctrl),
					AWSClient:                   aws.NewMockClient(ctrl),
				}

				testcase.ConfigureMocks(mocks)

				framework.KubernetesInterface = mocks.Kubernetes
				framework.NamespaceInterface = mocks.Namespace
				framework.Prompter = mocks.Prompter
				framework.HelmInterface = mocks.Helm
				framework.ConnectionFactory = &connections.MockFactory{
					ApplicationsManagementClient: mocks.ApplicationManagementClient,
				}
				framework.AzureClient = mocks.AzureClient
				framework.AWSClient = mocks.AWSClient
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
				require.NoError(t, err, "validation should have passed but it failed")
			} else if err != nil {
				// We expected this to fail, so it's OK if it does. No need to run Validate.
				return
			}

			err = validateRequiredFlags(cmd)
			if err != nil && testcase.ExpectedValid {
				require.NoError(t, err, "validation should have passed but it failed")
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

			if testcase.ValidateCallback != nil {
				testcase.ValidateCallback(t, runner)
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

// # Function Explanation
//
// LoadConfig reads a YAML configuration from a string and returns a Viper object.
func LoadConfig(t *testing.T, yamlData string) *viper.Viper {
	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(bytes.NewBuffer([]byte(yamlData)))
	require.NoError(t, err)
	return v
}

// # Function Explanation
//
// LoadConfigWithWorkspace loads a config with a workspace and returns a viper instance.
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

// # Function Explanation
//
// LoadConfigWithWorkspaceAndApplication loads a config with a test-workspace and test-application.
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

// # Function Explanation
//
// LoadEmptyConfig creates a viper instance with an empty workspaces configuration.
func LoadEmptyConfig(t *testing.T) *viper.Viper {

	var yamlData = `
workspaces: 
`

	return LoadConfig(t, yamlData)
}

// # Function Explanation
//
// Create404Error creates an error with a status code of 404.
func Create404Error() error {
	code := v1.CodeNotFound
	return &azcore.ResponseError{
		ErrorCode:  code,
		StatusCode: 404,
	}
}

// # Function Explanation
//
// CreateResource creates a generic resource with the given resource type and name, and sets the ID, Name, Type and
// Location fields.
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

// # Function Explanation
//
// // CreateResourceGroup creates a ResourceGroupResource object with the given name and a generated ID.
func CreateResourceGroup(resourceGroupName string) v20220901privatepreview.ResourceGroupResource {
	id := fmt.Sprintf("/planes/radius/local/resourcegroups/%s", resourceGroupName)
	return v20220901privatepreview.ResourceGroupResource{
		Name: &resourceGroupName,
		ID:   &id,
	}
}
