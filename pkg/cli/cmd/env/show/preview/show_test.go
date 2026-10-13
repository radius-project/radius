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

package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/test/radcli"
)

const testScope = "/planes/radius/local/resourceGroups/test-group"

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Show Command with default environment",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with flag",
			Input:         []string{"-e", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with positional arg",
			Input:         []string{"test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Show Command with fallback workspace",
			Input:         []string{"--environment", "test-env", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Show Command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run_JSONWritesEnvironmentOnly(t *testing.T) {
	providers := &corerpv20250801.Providers{
		Azure: &corerpv20250801.ProvidersAzure{
			SubscriptionID:    new("test-subscription-id"),
			ResourceGroupName: new("test-resource-group"),
		},
	}

	testcases := []struct {
		name        string
		providers   *corerpv20250801.Providers
		recipePacks []*string
	}{
		{
			name:      "providers only",
			providers: providers,
		},
		{
			name: "references only",
			recipePacks: []*string{
				new(testScope + "/providers/Radius.Core/recipePacks/default"),
			},
		},
		{
			name:      "providers and references",
			providers: providers,
			recipePacks: []*string{
				new("/planes/radius/local/resourceGroups/shared/providers/Radius.Core/recipePacks/default"),
			},
		},
		{
			name: "no providers or references",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			environment := corerpv20250801.EnvironmentResource{
				ID:       new(testScope + "/providers/Radius.Core/environments/test-env"),
				Name:     new("test-env"),
				Type:     new("Radius.Core/environments"),
				Location: new("global"),
				Tags:     map[string]*string{"team": new("platform")},
				Properties: &corerpv20250801.EnvironmentProperties{
					BicepSettings: new(testScope + "/providers/Radius.Core/bicepSettings/default"),
					Providers:     tc.providers,
					RecipePacks:   tc.recipePacks,
					Simulated:     new(true),
				},
			}
			recipePackGets := 0
			var stdout bytes.Buffer
			err := runShow(
				t,
				environmentServer(environment),
				forbiddenRecipePackServer(&recipePackGets),
				output.FormatJson,
				&stdout)
			require.NoError(t, err)

			var actual corerpv20250801.EnvironmentResource
			decoder := json.NewDecoder(&stdout)
			require.NoError(t, decoder.Decode(&actual))
			require.Equal(t, environment, actual)
			require.ErrorIs(t, decoder.Decode(&struct{}{}), io.EOF)
			require.Zero(t, recipePackGets)
		})
	}
}

func Test_Run_TableOutput(t *testing.T) {
	providers := &corerpv20250801.Providers{
		Azure: &corerpv20250801.ProvidersAzure{
			SubscriptionID:    new("test-subscription-id"),
			ResourceGroupName: new("test-resource-group"),
		},
		Aws: &corerpv20250801.ProvidersAws{
			AccountID: new("test-account-id"),
			Region:    new("test-region"),
		},
		Kubernetes: &corerpv20250801.ProvidersKubernetes{
			Namespace: new("test-namespace"),
		},
	}
	testcases := []struct {
		name              string
		recipePacks       []*string
		recipePackPattern string
	}{
		{
			name: "ordered recipe pack references",
			recipePacks: []*string{
				new("/planes/radius/local/resourceGroups/shared/providers/Radius.Core/recipePacks/default"),
				new("/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/recipePacks/pack-b"),
				new("/planes/radius/local/resourceGroups/another/providers/Radius.Core/recipePacks/default"),
			},
			recipePackPattern: `(?s)default\s+shared.*pack-b\s+test-group.*default\s+another`,
		},
		{
			name: "no recipe pack references",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			environment := corerpv20250801.EnvironmentResource{
				ID:   new(testScope + "/providers/Radius.Core/environments/test-env"),
				Name: new("test-env"),
				Type: new("Radius.Core/environments"),
				Properties: &corerpv20250801.EnvironmentProperties{
					ProvisioningState: new(corerpv20250801.ProvisioningStateSucceeded),
					Providers:         providers,
					RecipePacks:       tc.recipePacks,
				},
			}
			recipePackGets := 0
			var stdout bytes.Buffer
			err := runShow(
				t,
				environmentServer(environment),
				forbiddenRecipePackServer(&recipePackGets),
				output.FormatTable,
				&stdout)
			require.NoError(t, err)

			actual := stdout.String()
			require.Contains(t, actual, "STATE")
			require.Contains(t, actual, "Succeeded")
			require.Contains(t, actual, "PROVIDER")
			require.Contains(t, actual, "azure")
			require.Contains(t, actual, "subscriptionId: 'test-subscription-id', resourceGroupName: 'test-resource-group'")
			require.Contains(t, actual, "aws")
			require.Contains(t, actual, "accountId: 'test-account-id', region: 'test-region'")
			require.Contains(t, actual, "kubernetes")
			require.Contains(t, actual, "namespace: 'test-namespace'")
			if tc.recipePackPattern == "" {
				require.NotContains(t, actual, "RECIPE PACK")
			} else {
				require.Contains(t, actual, "RECIPE PACK")
				require.Regexp(t, tc.recipePackPattern, actual)
				require.NotContains(t, actual, "RESOURCE TYPE")
				require.NotContains(t, actual, "RECIPE KIND")
				require.NotContains(t, actual, "RECIPE SOURCE")
			}
			require.Zero(t, recipePackGets)
		})
	}
}

func Test_Run_TableReturnsInvalidRecipePackReferenceError(t *testing.T) {
	envServer := environmentServer(corerpv20250801.EnvironmentResource{
		ID:   new(testScope + "/providers/Radius.Core/environments/test-env"),
		Name: new("test-env"),
		Properties: &corerpv20250801.EnvironmentProperties{
			RecipePacks: []*string{new("not-a-resource-id")},
		},
	})

	var stdout bytes.Buffer
	require.Error(t, runShow(t, envServer, nil, output.FormatTable, &stdout))
	require.Empty(t, stdout.String())
}

func Test_Run_EnvironmentFetchErrors(t *testing.T) {
	testcases := []struct {
		name          string
		status        int
		expectedError error
	}{
		{
			name:          "not found",
			status:        http.StatusNotFound,
			expectedError: clierrors.Message("The environment %q does not exist. Please select a new environment and try again.", "test-env"),
		},
		{
			name:   "server error",
			status: http.StatusInternalServerError,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			envServer := func() fake.EnvironmentsServer {
				return fake.EnvironmentsServer{
					Get: func(
						ctx context.Context,
						rootScope string,
						environmentName string,
						options *corerpv20250801.EnvironmentsClientGetOptions,
					) (resp azfake.Responder[corerpv20250801.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
						errResp.SetResponseError(tc.status, http.StatusText(tc.status))
						return
					},
				}
			}

			var stdout bytes.Buffer
			err := runShow(t, envServer, nil, output.FormatTable, &stdout)
			require.Error(t, err)
			if tc.expectedError != nil {
				require.Equal(t, tc.expectedError, err)
			} else {
				var responseError *azcore.ResponseError
				require.ErrorAs(t, err, &responseError)
				require.Equal(t, tc.status, responseError.StatusCode)
			}
			require.Empty(t, stdout.String())
		})
	}
}

func Test_Run_PropagatesOutputErrors(t *testing.T) {
	environment := corerpv20250801.EnvironmentResource{
		ID:   new(testScope + "/providers/Radius.Core/environments/test-env"),
		Name: new("test-env"),
		Type: new("Radius.Core/environments"),
		Properties: &corerpv20250801.EnvironmentProperties{
			ProvisioningState: new(corerpv20250801.ProvisioningStateSucceeded),
			Providers: &corerpv20250801.Providers{
				Kubernetes: &corerpv20250801.ProvidersKubernetes{
					Namespace: new("test-namespace"),
				},
			},
			RecipePacks: []*string{
				new(testScope + "/providers/Radius.Core/recipePacks/default"),
			},
		},
	}

	testcases := []struct {
		name   string
		format string
		marker string
	}{
		{name: "json environment", format: output.FormatJson, marker: "{"},
		{name: "table environment", format: output.FormatTable, marker: "RESOURCE"},
		{name: "table providers", format: output.FormatTable, marker: "PROVIDER"},
		{name: "table recipe packs", format: output.FormatTable, marker: "RECIPE PACK"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			writeError := errors.New("write failed")
			err := runShow(
				t,
				environmentServer(environment),
				nil,
				tc.format,
				&failOnTextWriter{
					marker: tc.marker,
					err:    writeError,
				})

			require.ErrorIs(t, err, writeError)
		})
	}
}

func runShow(
	t *testing.T,
	envServer func() fake.EnvironmentsServer,
	recipePackServer func() fake.RecipePacksServer,
	format string,
	writer io.Writer,
) error {
	t.Helper()

	factory, err := test_client_factory.NewRadiusCoreTestClientFactory(testScope, envServer, recipePackServer)
	require.NoError(t, err)

	runner := &Runner{
		RadiusCoreClientFactory: factory,
		Workspace: &workspaces.Workspace{
			Name:  "test-workspace",
			Scope: testScope,
		},
		EnvironmentName: "test-env",
		Format:          format,
		Output:          &output.OutputWriter{Writer: writer},
	}

	return runner.Run(t.Context())
}

func forbiddenRecipePackServer(gets *int) func() fake.RecipePacksServer {
	return func() fake.RecipePacksServer {
		return fake.RecipePacksServer{
			Get: func(
				ctx context.Context,
				rootScope string,
				recipePackName string,
				options *corerpv20250801.RecipePacksClientGetOptions,
			) (resp azfake.Responder[corerpv20250801.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
				(*gets)++
				errResp.SetResponseError(http.StatusForbidden, "Forbidden")
				return
			},
		}
	}
}

func environmentServer(environment corerpv20250801.EnvironmentResource) func() fake.EnvironmentsServer {
	return func() fake.EnvironmentsServer {
		return fake.EnvironmentsServer{
			Get: func(
				ctx context.Context,
				rootScope string,
				environmentName string,
				options *corerpv20250801.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[corerpv20250801.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				resp.SetResponse(http.StatusOK, corerpv20250801.EnvironmentsClientGetResponse{
					EnvironmentResource: environment,
				}, nil)
				return
			},
		}
	}
}

type failOnTextWriter struct {
	written bytes.Buffer
	marker  string
	err     error
}

func (w *failOnTextWriter) Write(p []byte) (int, error) {
	if strings.Contains(w.written.String()+string(p), w.marker) {
		return 0, w.err
	}

	return w.written.Write(p)
}
