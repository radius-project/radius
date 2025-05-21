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

package processor

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/stretchr/testify/require"
)

func Test_Process(t *testing.T) {
	processor := DynamicProcessor{}
	clientFactory, err := testUCPClientFactory()
	require.NoError(t, err)
	hostname := "test-hostname"
	port := 1234
	database := "test-db"
	username := "test-user"
	password := "test-password"
	environment := "test-environment"
	application := "test-application"
	t.Run("success", func(t *testing.T) {
		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource",
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{
					UpdatedAPIVersion: "2024-01-01",
				},
			},
			Properties: map[string]any{
				"status": map[string]any{},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					"/planes/kubernetes/local/namespaces/test-ns/providers/core/Service/test-svc",
				},
				Values: map[string]any{
					"host":     hostname,
					"port":     float64(port),
					"database": database,
					"username": username,
				},
				Secrets: map[string]any{
					"password": password,
				},
			},
			UcpClient: clientFactory,
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		bs, err := json.Marshal(resource.Properties)
		require.NoError(t, err)

		properties := map[string]any{}
		err = json.Unmarshal(bs, &properties)
		require.NoError(t, err)

		require.Equal(t, options.RecipeOutput.Values["host"], properties["host"])
		require.Equal(t, options.RecipeOutput.Values["port"], properties["port"])
		require.Equal(t, options.RecipeOutput.Values["database"], properties["database"])
		require.Equal(t, options.RecipeOutput.Values["username"], properties["username"])

		// password property is not defined in the schema but present as part of the recipe output.
		// so, it is not added to the resource properties but instead available in  properties.status.computedValues and properties.status.secrets maps.
		_, ok := properties["password"]
		require.False(t, ok)

		status, ok := properties["status"].(map[string]any)
		require.True(t, ok)

		computedValues, ok := status["computedValues"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, options.RecipeOutput.Values["host"], computedValues["host"])
		require.Equal(t, options.RecipeOutput.Values["port"], computedValues["port"])
		require.Equal(t, options.RecipeOutput.Values["database"], computedValues["database"])
		require.Equal(t, options.RecipeOutput.Values["username"], computedValues["username"])

		secrets, ok := status["secrets"].(map[string]any)
		require.True(t, ok)

		secretPassword, ok := secrets["password"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, options.RecipeOutput.Secrets["password"], secretPassword["Value"])
	})

	// test to check if the properties like environment, application , status etc are not overwritten if they are provided as part of the recipe output.
	t.Run("do not overwite basic properties", func(t *testing.T) {
		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource",
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{
					UpdatedAPIVersion: "2024-01-01",
				},
			},
			Properties: map[string]any{
				"environment": environment,
				"application": application,
				"status":      map[string]any{},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					"/planes/kubernetes/local/namespaces/test-ns/providers/core/Service/test-svc",
				},
				Values: map[string]any{
					"host":        hostname,
					"port":        float64(port),
					"database":    database,
					"username":    username,
					"environment": "overwrite-environment",
					"application": "overwrite-application",
				},
				Secrets: map[string]any{
					"password": password,
				},
			},
			UcpClient: clientFactory,
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		bs, err := json.Marshal(resource.Properties)
		require.NoError(t, err)

		properties := map[string]any{}
		err = json.Unmarshal(bs, &properties)
		require.NoError(t, err)

		require.Equal(t, environment, properties["environment"])
		require.Equal(t, application, properties["application"])
	})

	t.Run("invalid resource id", func(t *testing.T) {
		resource := &datamodel.DynamicResource{}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					"/planes/kubernetes/local/namespaces/test-ns/providers/core/Service/test-svc",
				},
				Values: map[string]any{
					"host":        hostname,
					"port":        float64(port),
					"database":    database,
					"username":    username,
					"environment": "overwrite-environment",
					"application": "overwrite-application",
				},
				Secrets: map[string]any{
					"password": password,
				},
			},
			UcpClient: clientFactory,
		}

		err := processor.Process(context.Background(), resource, options)
		require.Error(t, err)
		require.Contains(t, err.Error(), "is not a valid resource id")
	})
}

func testUCPClientFactory() (*v20231001preview.ClientFactory, error) {
	apiVersionServer := fake.APIVersionsServer{
		Get: func(ctx context.Context, planeName, resourceProviderName, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.APIVersionsClientGetResponse{
				APIVersionResource: v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{
						Schema: map[string]any{
							"properties": map[string]any{
								"environment": map[string]any{},
								"application": map[string]any{},
								"host":        map[string]any{},
								"database": map[string]any{
									connectedResourceOutputVariable: "PG_DB",
								},
								"port":     map[string]any{},
								"username": map[string]any{},
							},
						},
					},
				},
			}

			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}

	return v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
				APIVersionsServer: apiVersionServer,
			}),
		},
	})
}
