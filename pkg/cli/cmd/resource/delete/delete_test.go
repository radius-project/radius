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

package delete

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	v20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid Delete Command",
			Input:         []string{"Applications.Core/containers", "foo"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with fallback workspace",
			Input:         []string{"Applications.Core/containers", "foo", "-g", "my-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "Delete Command with invalid resource type",
			Input:         []string{"invalidResourceType", "foo"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with insufficient args",
			Input:         []string{"Applications.Core/containers"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Delete Command with too many args",
			Input:         []string{"Applications.Core/containers", "a", "b"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with ambiguous args",
			Input:         []string{"Applications.Core/secretStores"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Delete resource", func(t *testing.T) {
		t.Run("Failure (non-existent resource type)", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			responseError := runtime.NewResponseError(
				&http.Response{
					Status:     "400 Bad Request",
					StatusCode: http.StatusBadRequest,
					Body:       http.NoBody,
					Request: &http.Request{
						Method: http.MethodGet,
						URL:    &url.URL{Path: "url"},
					},
				})
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Foo.Bar/myType", "test").
				Return(generated.GenericResource{}, responseError)
			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         outputSink,
				Workspace:                      &workspaces.Workspace{},
				FullyQualifiedResourceTypeName: "Foo.Bar/myType",
				ResourceName:                   "test",
				Format:                         "table",
				Confirm:                        true,
			}
			err := runner.Run(t.Context())
			require.Error(t, err)
			require.Equal(t, responseError, err)
		})

		t.Run("Success (non-existent)", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]any{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)

			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container", false).
				Return(false, nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         outputSink,
				Workspace:                      &workspaces.Workspace{},
				FullyQualifiedResourceTypeName: "Applications.Core/containers",
				ResourceName:                   "test-container",
				Format:                         "table",
				Confirm:                        true,
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "%s/%s not found",
					Params: []any{"Applications.Core/containers", "test-container"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success (deleted)", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]any{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container", false).
				Return(true, nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         outputSink,
				Workspace:                      &workspaces.Workspace{},
				FullyQualifiedResourceTypeName: "Applications.Core/containers",
				ResourceName:                   "test-container",
				Format:                         "table",
				Confirm:                        true,
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "%s/%s deleted",
					Params: []any{"Applications.Core/containers", "test-container"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success: Prompt Confirmed (case 1: application-scoped standard resource)", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithApplication, "test-container", "Applications.Core/containers", "my-test-app", "my-test-env")).
				Return(prompt.ConfirmYes, nil).
				Times(1)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]any{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container", false).
				Return(true, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}
			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         outputSink,
				Workspace:                      workspace,
				FullyQualifiedResourceTypeName: "Applications.Core/containers",
				ResourceName:                   "test-container",
				Format:                         "table",
				InputPrompter:                  promptMock,
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "%s/%s deleted",
					Params: []any{"Applications.Core/containers", "test-container"},
				},
			}

			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success: Prompt Confirmed (case 2: environment-scoped standard resource)", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithoutApplication, "test-container", "Applications.Core/containers", "my-test-env")).
				Return(prompt.ConfirmYes, nil).
				Times(1)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]any{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
					},
				}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container", false).
				Return(true, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         outputSink,
				Workspace:                      workspace,
				FullyQualifiedResourceTypeName: "Applications.Core/containers",
				ResourceName:                   "test-container",
				Format:                         "table",
				InputPrompter:                  promptMock,
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "%s/%s deleted",
					Params: []any{"Applications.Core/containers", "test-container"},
				},
			}

			require.Equal(t, expected, outputSink.Writes)
		})

		// NOTE: this case requires an extra lookup to get the environment name.
		t.Run("Success: Prompt Confirmed (case 3: application-scoped core resource)", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithApplication, "test-container", "Applications.Core/containers", "my-test-app", "my-test-env")).
				Return(prompt.ConfirmYes, nil).
				Times(1)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]any{
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)

			appManagementClient.EXPECT().
				GetApplication(gomock.Any(), "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app").
				Return(v20231001preview.ApplicationResource{
					Properties: &v20231001preview.ApplicationProperties{
						Environment: to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env"),
					},
				}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container", false).
				Return(true, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         outputSink,
				Workspace:                      workspace,
				FullyQualifiedResourceTypeName: "Applications.Core/containers",
				ResourceName:                   "test-container",
				Format:                         "table",
				InputPrompter:                  promptMock,
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "%s/%s deleted",
					Params: []any{"Applications.Core/containers", "test-container"},
				},
			}

			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success: Prompt Confirmed (case 4: no application or environment)", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithoutApplicationOrEnvironment, "test-container", "Applications.Core/containers")).
				Return(prompt.ConfirmYes, nil).
				Times(1)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]any{},
				}, nil).
				Times(1)

			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container", false).
				Return(true, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}
			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         outputSink,
				Workspace:                      workspace,
				FullyQualifiedResourceTypeName: "Applications.Core/containers",
				ResourceName:                   "test-container",
				Format:                         "table",
				InputPrompter:                  promptMock,
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "%s/%s deleted",
					Params: []any{"Applications.Core/containers", "test-container"},
				},
			}

			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("Success: Prompt Cancelled", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]any{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)

			promptMock := prompt.NewMockInterface(ctrl)
			promptMock.EXPECT().
				GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, fmt.Sprintf(deleteConfirmationWithApplication, "test-container", "Applications.Core/containers", "my-test-app", "my-test-env")).
				Return(prompt.ConfirmNo, nil).
				Times(1)

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				InputPrompter:                  promptMock,
				Workspace:                      workspace,
				Format:                         "table",
				Output:                         outputSink,
				FullyQualifiedResourceTypeName: "Applications.Core/containers",
				ResourceName:                   "test-container",
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			require.Empty(t, outputSink.Writes)
		})

		t.Run("Success (force deleted)", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Applications.Core/containers", "test-container").
				Return(generated.GenericResource{
					Properties: map[string]any{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/my-test-app",
					},
				}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Applications.Core/containers", "test-container", true).
				Return(true, nil).
				Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         outputSink,
				Workspace:                      &workspaces.Workspace{},
				FullyQualifiedResourceTypeName: "Applications.Core/containers",
				ResourceName:                   "test-container",
				Format:                         "table",
				Confirm:                        true,
				Force:                          true,
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.LogOutput{
					Format: "WARNING: Force deleting a resource in a non-terminal state may leave orphaned external resources that require manual cleanup.",
				},
				output.LogOutput{
					Format: "%s/%s deleted",
					Params: []any{"Applications.Core/containers", "test-container"},
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})

		// The resource's Terraform recipe output and another resource's Bicep recipe output refer to
		// the same S3 bucket under different output resource IDs. The warning names the other
		// resource, is shown before the prompt, and does not stop the delete.
		for _, confirm := range []bool{false, true} {
			t.Run(fmt.Sprintf("Success: warns about shared output resources and still deletes (yes=%v)", confirm), func(t *testing.T) {
				ctrl := gomock.NewController(t)

				const bucketARN = "arn:aws:s3:::shared-bucket"
				resourceID := "/planes/radius/local/resourceGroups/test-group/providers/Radius.Storage/buckets/data"
				otherResourceID := "/planes/radius/local/resourceGroups/other-group/providers/Radius.Storage/buckets/data"
				resource := generated.GenericResource{
					ID: &resourceID,
					Properties: map[string]any{
						"environment": "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/my-test-env",
						"application": "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/applications/my-test-app",
						"status": map[string]any{"outputResources": []any{
							map[string]any{"id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket", "providerResourceId": bucketARN, "radiusManaged": true},
						}},
					},
				}
				otherResource := generated.GenericResource{
					ID: &otherResourceID,
					Properties: map[string]any{"status": map[string]any{"outputResources": []any{
						map[string]any{"id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket", "providerResourceId": bucketARN, "radiusManaged": true},
					}}},
				}

				appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
				appManagementClient.EXPECT().
					GetResource(gomock.Any(), "Radius.Storage/buckets", "data").
					Return(resource, nil).
					Times(1)
				appManagementClient.EXPECT().
					ListResourcesInPlane(gomock.Any()).
					Return([]generated.GenericResource{resource, otherResource}, nil).
					Times(1)
				appManagementClient.EXPECT().
					DeleteResource(gomock.Any(), "Radius.Storage/buckets", "data", false).
					Return(true, nil).
					Times(1)

				outputSink := &output.MockOutput{}
				var logsAtPromptTime []any
				promptMock := prompt.NewMockInterface(ctrl)
				if !confirm {
					promptMock.EXPECT().
						GetListInput(gomock.Any(), gomock.Any()).
						DoAndReturn(func(choices []string, promptMsg string) (string, error) {
							logsAtPromptTime = append([]any{}, outputSink.Writes...)
							return prompt.ConfirmYes, nil
						}).
						Times(1)
				}

				runner := &Runner{
					ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
					Output:                         outputSink,
					InputPrompter:                  promptMock,
					Workspace:                      &workspaces.Workspace{},
					FullyQualifiedResourceTypeName: "Radius.Storage/buckets",
					ResourceName:                   "data",
					Format:                         "table",
					Confirm:                        confirm,
				}

				err := runner.Run(t.Context())
				require.NoError(t, err)

				warning := []any{
					output.LogOutput{Format: "WARNING: Deleting %s also deletes output resources that other Radius resources use:", Params: []any{"Radius.Storage/buckets/data"}},
					output.LogOutput{Format: "  - %s, also used by %s", Params: []any{bucketARN, otherResourceID}},
				}
				expected := append(append([]any{}, warning...), output.LogOutput{
					Format: "%s/%s deleted",
					Params: []any{"Radius.Storage/buckets", "data"},
				})
				require.Equal(t, expected, outputSink.Writes)
				if !confirm {
					require.Equal(t, warning, logsAtPromptTime)
				}
			})
		}

		// A different bucket elsewhere gives no warning, and a failed check logs a warning. Both
		// still delete the resource.
		for _, tc := range []struct {
			name           string
			otherARN       string
			listErr        error
			expectedWrites []any
		}{
			{name: "no match", otherARN: "arn:aws:s3:::other-bucket"},
			{
				name:    "listing fails",
				listErr: fmt.Errorf("list failed"),
				expectedWrites: []any{
					output.LogOutput{Format: "WARNING: Could not check whether other Radius resources use the output resources of %s: %v", Params: []any{"Radius.Storage/buckets/data", fmt.Errorf("list failed")}},
				},
			},
		} {
			t.Run("Success: deletes when "+tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)

				resourceID := "/planes/radius/local/resourceGroups/test-group/providers/Radius.Storage/buckets/data"
				otherResourceID := "/planes/radius/local/resourceGroups/other-group/providers/Radius.Storage/buckets/data"
				resource := generated.GenericResource{
					ID: &resourceID,
					Properties: map[string]any{"status": map[string]any{"outputResources": []any{
						map[string]any{"id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket", "providerResourceId": "arn:aws:s3:::shared-bucket", "radiusManaged": true},
					}}},
				}
				otherResource := generated.GenericResource{
					ID: &otherResourceID,
					Properties: map[string]any{"status": map[string]any{"outputResources": []any{
						map[string]any{"id": "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/other-bucket", "providerResourceId": tc.otherARN, "radiusManaged": true},
					}}},
				}

				appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
				appManagementClient.EXPECT().
					GetResource(gomock.Any(), "Radius.Storage/buckets", "data").
					Return(resource, nil).
					Times(1)
				if tc.listErr != nil {
					appManagementClient.EXPECT().ListResourcesInPlane(gomock.Any()).Return(nil, tc.listErr).Times(1)
				} else {
					appManagementClient.EXPECT().ListResourcesInPlane(gomock.Any()).Return([]generated.GenericResource{resource, otherResource}, nil).Times(1)
				}
				appManagementClient.EXPECT().
					DeleteResource(gomock.Any(), "Radius.Storage/buckets", "data", false).
					Return(true, nil).
					Times(1)

				outputSink := &output.MockOutput{}
				runner := &Runner{
					ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
					Output:                         outputSink,
					Workspace:                      &workspaces.Workspace{},
					FullyQualifiedResourceTypeName: "Radius.Storage/buckets",
					ResourceName:                   "data",
					Format:                         "table",
					Confirm:                        true,
				}

				err := runner.Run(t.Context())
				require.NoError(t, err)

				expected := append(append([]any{}, tc.expectedWrites...), output.LogOutput{
					Format: "%s/%s deleted",
					Params: []any{"Radius.Storage/buckets", "data"},
				})
				require.Equal(t, expected, outputSink.Writes)
			})
		}

		t.Run("Success: names every owner of a provider resource listed under two output resource IDs", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			const bucketARN = "arn:aws:s3:::shared-bucket"
			bicepID := "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/shared-bucket"
			terraformID := "/planes/aws/aws/accounts/123456789012/regions/global/providers/Terraform.AWS/aws_s3_bucket/shared-bucket"
			resourceID := "/planes/radius/local/resourceGroups/team-a/providers/Radius.Storage/buckets/data"
			teamB := "/planes/radius/local/resourceGroups/team-b/providers/Radius.Storage/buckets/data"
			teamC := "/planes/radius/local/resourceGroups/team-c/providers/Radius.Storage/buckets/data"
			resource := generated.GenericResource{
				ID: &resourceID,
				Properties: map[string]any{"status": map[string]any{"outputResources": []any{
					map[string]any{"id": bicepID, "providerResourceId": bucketARN, "radiusManaged": true},
					map[string]any{"id": terraformID, "providerResourceId": bucketARN, "radiusManaged": true},
				}}},
			}
			// Older references without provider resource IDs, one for each output resource ID.
			reference := func(id string, outputResourceID string) generated.GenericResource {
				return generated.GenericResource{
					ID: new(id),
					Properties: map[string]any{"status": map[string]any{"outputResources": []any{
						map[string]any{"id": outputResourceID, "radiusManaged": false},
					}}},
				}
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Radius.Storage/buckets", "data").
				Return(resource, nil).
				Times(1)
			appManagementClient.EXPECT().
				ListResourcesInPlane(gomock.Any()).
				Return([]generated.GenericResource{resource, reference(teamB, bicepID), reference(teamC, terraformID)}, nil).
				Times(1)
			appManagementClient.EXPECT().
				DeleteResource(gomock.Any(), "Radius.Storage/buckets", "data", false).
				Return(true, nil).
				Times(1)

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         outputSink,
				Workspace:                      &workspaces.Workspace{},
				FullyQualifiedResourceTypeName: "Radius.Storage/buckets",
				ResourceName:                   "data",
				Format:                         "table",
				Confirm:                        true,
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			require.Equal(t, []any{
				output.LogOutput{Format: "WARNING: Deleting %s also deletes output resources that other Radius resources use:", Params: []any{"Radius.Storage/buckets/data"}},
				output.LogOutput{Format: "  - %s, also used by %s", Params: []any{bucketARN, teamB + ", " + teamC}},
				output.LogOutput{Format: "%s/%s deleted", Params: []any{"Radius.Storage/buckets", "data"}},
			}, outputSink.Writes)
		})

		t.Run("Failure: an environment that is not a resource ID stops the delete", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// No DeleteResource or ListResourcesInPlane expectation: the mock fails the test if either is called.
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResource(gomock.Any(), "Radius.Storage/buckets", "data").
				Return(generated.GenericResource{Properties: map[string]any{"environment": 42}}, nil).
				Times(1)

			runner := &Runner{
				ConnectionFactory:              &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                         &output.MockOutput{},
				Workspace:                      &workspaces.Workspace{},
				FullyQualifiedResourceTypeName: "Radius.Storage/buckets",
				ResourceName:                   "data",
				Format:                         "table",
				Confirm:                        true,
			}

			err := runner.Run(t.Context())
			require.EqualError(t, err, "resource ID is not a string")
		})
	})

}
