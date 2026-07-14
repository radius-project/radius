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
	"fmt"
	"net/http"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/dynamicrp/backend/secret"
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

		// password is a recipe secret output, but this resource type does not declare a secrets block,
		// so it is dropped: never copied to properties and never persisted in status.
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

		// Secret outputs are never persisted on the owner resource.
		_, hasSecrets := status["secrets"]
		require.False(t, hasSecrets, "secret outputs must not be stored in status")

		// No secrets block is declared, so no managed secret reference is set.
		_, hasSecretRef := properties["secrets"]
		require.False(t, hasSecretRef)
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

	t.Run("materializes declared secret outputs into a managed secret", func(t *testing.T) {
		mat := &fakeMaterializer{result: secret.Result{
			ID:   "/planes/radius/local/resourceGroups/test-group/providers/Radius.Security/secrets/test-resource-secrets",
			Name: "test-resource-secrets",
		}}
		p := DynamicProcessor{SecretMaterializer: mat}
		cf, err := testUCPClientFactoryWithSecrets("connectionString")
		require.NoError(t, err)

		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource",
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{UpdatedAPIVersion: "2024-01-01"},
			},
			Properties: map[string]any{
				"environment": environment,
				"application": application,
				"status":      map[string]any{},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Values:  map[string]any{"host": hostname},
				Secrets: map[string]any{"connectionString": "secret-conn"},
			},
			UcpClient: cf,
		}

		require.NoError(t, p.Process(context.Background(), resource, options))

		// The managed secret is created with the declared secret data and the owner's environment/application.
		require.True(t, mat.called)
		require.Equal(t, resource.ID, mat.request.OwnerResourceID)
		require.Equal(t, environment, mat.request.EnvironmentID)
		require.Equal(t, application, mat.request.ApplicationID)
		require.Equal(t, map[string]string{"connectionString": "secret-conn"}, mat.request.Data)

		bs, err := json.Marshal(resource.Properties)
		require.NoError(t, err)
		properties := map[string]any{}
		require.NoError(t, json.Unmarshal(bs, &properties))

		// The secret value is never stored on the owner, and the non-secret computed value still is.
		_, hasPlaintext := properties["connectionString"]
		require.False(t, hasPlaintext)
		require.Equal(t, hostname, properties["host"])

		// The owner exposes only the managed secret's name via the reserved `secrets.name` reference.
		// The declared secret data key is never populated on the owner, and the id is not exposed.
		secrets, ok := properties["secrets"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "test-resource-secrets", secrets["name"])
		require.NotContains(t, secrets, "id")
		require.NotContains(t, secrets, "connectionString")

		status, _ := properties["status"].(map[string]any)
		_, hasSecrets := status["secrets"]
		require.False(t, hasSecrets)
	})

	t.Run("materializes recipe secret outputs not declared in the schema secrets block", func(t *testing.T) {
		mat := &fakeMaterializer{result: secret.Result{
			ID:   "/planes/radius/local/resourceGroups/test-group/providers/Radius.Security/secrets/test-resource-secrets",
			Name: "test-resource-secrets",
		}}
		p := DynamicProcessor{SecretMaterializer: mat}
		// The schema's secrets block declares only `connectionString`; the recipe also emits `extraSecret`.
		cf, err := testUCPClientFactoryWithSecrets("connectionString")
		require.NoError(t, err)

		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource",
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{UpdatedAPIVersion: "2024-01-01"},
			},
			Properties: map[string]any{"status": map[string]any{}},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Values:  map[string]any{"host": hostname},
				Secrets: map[string]any{"connectionString": "secret-conn", "extraSecret": "secret-extra"},
			},
			UcpClient: cf,
		}

		require.NoError(t, p.Process(context.Background(), resource, options))

		// Both the declared and the undeclared recipe secret outputs are materialized: the schema's declared
		// keys document the expected surface but do not filter what is routed to the managed secret, so a
		// recipe-emitted secret the type author did not anticipate is not silently dropped.
		require.True(t, mat.called)
		require.Equal(t, map[string]string{"connectionString": "secret-conn", "extraSecret": "secret-extra"}, mat.request.Data)

		// Neither secret value lands on the owner; only the managed secret name reference is exposed.
		bs, err := json.Marshal(resource.Properties)
		require.NoError(t, err)
		properties := map[string]any{}
		require.NoError(t, json.Unmarshal(bs, &properties))
		require.NotContains(t, properties, "connectionString")
		require.NotContains(t, properties, "extraSecret")
		secrets, ok := properties["secrets"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "test-resource-secrets", secrets["name"])
	})

	t.Run("stringifies non-string secret values before materializing", func(t *testing.T) {
		mat := &fakeMaterializer{result: secret.Result{ID: "managed-id", Name: "managed-name"}}
		p := DynamicProcessor{SecretMaterializer: mat}
		cf, err := testUCPClientFactoryWithSecrets("port", "enabled")
		require.NoError(t, err)

		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource",
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{UpdatedAPIVersion: "2024-01-01"},
			},
			Properties: map[string]any{"status": map[string]any{}},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Values: map[string]any{"host": hostname},
				// A direct module may emit sensitive outputs that are not strings (numbers, booleans).
				Secrets: map[string]any{"port": float64(port), "enabled": true},
			},
			UcpClient: cf,
		}

		require.NoError(t, p.Process(context.Background(), resource, options))

		// The declared secret values are stringified and routed to the managed secret, not the owner.
		require.Equal(t, map[string]string{"port": "1234", "enabled": "true"}, mat.request.Data)

		bs, err := json.Marshal(resource.Properties)
		require.NoError(t, err)
		properties := map[string]any{}
		require.NoError(t, json.Unmarshal(bs, &properties))
		status, _ := properties["status"].(map[string]any)
		_, hasSecrets := status["secrets"]
		require.False(t, hasSecrets)
	})

	t.Run("nil secret values are skipped", func(t *testing.T) {
		mat := &fakeMaterializer{result: secret.Result{ID: "managed-id", Name: "managed-name"}}
		p := DynamicProcessor{SecretMaterializer: mat}
		cf, err := testUCPClientFactoryWithSecrets("password")
		require.NoError(t, err)

		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource",
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{UpdatedAPIVersion: "2024-01-01"},
			},
			Properties: map[string]any{"status": map[string]any{}},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Values: map[string]any{"host": hostname},
				// A nil secret output must not be recorded as the literal string "<nil>".
				Secrets: map[string]any{"password": nil},
			},
			UcpClient: cf,
		}

		require.NoError(t, p.Process(context.Background(), resource, options))

		// With only a nil secret value, there is no secret data to materialize.
		require.False(t, mat.called, "materializer should not be called when there is no secret data")
		_, hasSecretRef := resource.Properties["secrets"]
		require.False(t, hasSecretRef)
	})

	t.Run("clears a stale managed secret when an update produces no secret outputs", func(t *testing.T) {
		mat := &fakeMaterializer{}
		p := DynamicProcessor{SecretMaterializer: mat}
		// The type still declares a secrets block, but this recipe run emits no secret outputs.
		cf, err := testUCPClientFactoryWithSecrets("connectionString")
		require.NoError(t, err)

		resourceID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource"
		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   resourceID,
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{UpdatedAPIVersion: "2024-01-01"},
			},
			// A prior deploy materialized a managed secret and left the reference behind.
			Properties: map[string]any{
				"status":  map[string]any{},
				"secrets": map[string]any{"name": "test-resource-secrets"},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Values:  map[string]any{"host": hostname},
				Secrets: map[string]any{},
			},
			UcpClient: cf,
		}

		require.NoError(t, p.Process(context.Background(), resource, options))

		// The now-stale managed secret is cascade-deleted and the dangling reference is cleared.
		require.Equal(t, []string{resourceID}, mat.deleted, "the stale managed secret should be deleted")
		require.False(t, mat.called, "no new secret should be materialized")
		_, hasSecretRef := resource.Properties["secrets"]
		require.False(t, hasSecretRef, "the stale secrets.name reference should be removed")
	})

	t.Run("clears a stale managed secret even when the schema no longer declares a secrets block", func(t *testing.T) {
		mat := &fakeMaterializer{}
		p := DynamicProcessor{SecretMaterializer: mat}
		// The type's schema no longer declares a secrets block, yet the owner still carries a reference
		// from a prior deploy that materialized one.
		cf, err := testUCPClientFactory()
		require.NoError(t, err)

		resourceID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource"
		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   resourceID,
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{UpdatedAPIVersion: "2024-01-01"},
			},
			Properties: map[string]any{
				"status":  map[string]any{},
				"secrets": map[string]any{"name": "test-resource-secrets"},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Values:  map[string]any{"host": hostname},
				Secrets: map[string]any{},
			},
			UcpClient: cf,
		}

		require.NoError(t, p.Process(context.Background(), resource, options))

		// Cleanup keys off the owner's reference, not the schema, so the orphan is still reclaimed.
		require.Equal(t, []string{resourceID}, mat.deleted, "the stale managed secret should be deleted")
		require.False(t, mat.called, "no new secret should be materialized")
		_, hasSecretRef := resource.Properties["secrets"]
		require.False(t, hasSecretRef, "the stale secrets.name reference should be removed")
	})

	t.Run("reclaims a stale managed secret when the block is dropped but the recipe still emits secrets", func(t *testing.T) {
		mat := &fakeMaterializer{}
		p := DynamicProcessor{SecretMaterializer: mat}
		// The type's schema no longer declares a secrets block, yet the recipe still emits secret outputs
		// and the owner still carries a reference from a prior deploy that materialized one.
		cf, err := testUCPClientFactory()
		require.NoError(t, err)

		resourceID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource"
		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   resourceID,
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{UpdatedAPIVersion: "2024-01-01"},
			},
			Properties: map[string]any{
				"status":  map[string]any{},
				"secrets": map[string]any{"name": "test-resource-secrets"},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Values: map[string]any{"host": hostname},
				// The recipe still emits a secret, but the type no longer opts into materialization.
				Secrets: map[string]any{"connectionString": "secret-conn"},
			},
			UcpClient: cf,
		}

		require.NoError(t, p.Process(context.Background(), resource, options))

		// The new secret is dropped (no block), and the prior managed secret is reclaimed rather than
		// orphaned — even though this update produced secret outputs.
		require.Equal(t, []string{resourceID}, mat.deleted, "the stale managed secret should be deleted")
		require.False(t, mat.called, "no new secret should be materialized without a secrets block")
		_, hasSecretRef := resource.Properties["secrets"]
		require.False(t, hasSecretRef, "the stale secrets.name reference should be removed")
	})
	t.Run("fails fast when the schema declares a malformed secrets block", func(t *testing.T) {
		mat := &fakeMaterializer{}
		p := DynamicProcessor{SecretMaterializer: mat}
		// `secrets` is declared as a string rather than the framework-owned object shape.
		malformed := map[string]any{
			"properties": map[string]any{
				"environment": map[string]any{},
				"application": map[string]any{},
				"secrets":     map[string]any{"type": "string"},
			},
		}
		cf, err := testUCPClientFactoryWithSchema(malformed)
		require.NoError(t, err)

		resource := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testRecipeResources/test-resource",
					Type: "Applications.Test/testRecipeResources",
				},
				InternalMetadata: v1.InternalMetadata{UpdatedAPIVersion: "2024-01-01"},
			},
			Properties: map[string]any{"status": map[string]any{}},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Values:  map[string]any{"host": hostname},
				Secrets: map[string]any{"connectionString": "secret-conn"},
			},
			UcpClient: cf,
		}

		err = p.Process(context.Background(), resource, options)
		require.Error(t, err, "a malformed secrets block should fail processing")
		require.Contains(t, err.Error(), "must be an object")
		require.False(t, mat.called, "no secret should be materialized for an invalid schema")
	})
}

// fakeMaterializer is a test double for secret.Materializer.
type fakeMaterializer struct {
	called  bool
	request secret.Request
	result  secret.Result
	err     error
	deleted []string
}

func (f *fakeMaterializer) Materialize(ctx context.Context, req secret.Request) (secret.Result, error) {
	f.called = true
	f.request = req
	return f.result, f.err
}

func (f *fakeMaterializer) Delete(ctx context.Context, ownerResourceID string) error {
	f.deleted = append(f.deleted, ownerResourceID)
	return f.err
}

// schemaWithSecretKeys builds a resource type schema that declares a secrets block containing the given
// secret keys, alongside the usual base properties.
func schemaWithSecretKeys(keys ...string) map[string]any {
	secretProps := map[string]any{}
	for _, key := range keys {
		secretProps[key] = map[string]any{"type": "string", "readOnly": true}
	}
	return map[string]any{
		"properties": map[string]any{
			"environment": map[string]any{},
			"application": map[string]any{},
			"host":        map[string]any{},
			"secrets": map[string]any{
				"type":       "object",
				"readOnly":   true,
				"properties": secretProps,
			},
		},
	}
}

// testUCPClientFactoryWithSecrets returns a client factory whose API version schema declares a secrets block
// with the provided keys.
func testUCPClientFactoryWithSecrets(keys ...string) (*v20231001preview.ClientFactory, error) {
	return testUCPClientFactoryWithSchema(schemaWithSecretKeys(keys...))
}

// testUCPClientFactoryWithSchema returns a client factory whose API version schema is the provided schema.
func testUCPClientFactoryWithSchema(schema map[string]any) (*v20231001preview.ClientFactory, error) {
	apiVersionServer := fake.APIVersionsServer{
		Get: func(ctx context.Context, planeName, resourceProviderName, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.APIVersionsClientGetResponse{
				APIVersionResource: v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{
						Schema: schema,
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
								"database":    map[string]any{},
								"port":        map[string]any{},
								"username":    map[string]any{},
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

func TestGetSchemaForResourceType(t *testing.T) {
	t.Run("success - schema found", func(t *testing.T) {
		clientFactory, err := testUCPClientFactory()
		require.NoError(t, err)

		ctx := context.Background()
		resourceType := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/test-resource"
		apiVersion := "2023-10-01-preview"

		schema, err := GetSchemaForResourceType(ctx, clientFactory, resourceType, apiVersion)
		require.NoError(t, err)
		require.NotNil(t, schema)

		schemaMap, ok := schema.(map[string]any)
		require.True(t, ok)

		properties, exists := schemaMap["properties"]
		require.True(t, exists)
		require.NotNil(t, properties)
	})

	t.Run("error - invalid resource type format", func(t *testing.T) {
		clientFactory, err := testUCPClientFactory()
		require.NoError(t, err)

		ctx := context.Background()
		resourceType := "invalid-resource-type-format"
		apiVersion := "2023-10-01-preview"

		schema, err := GetSchemaForResourceType(ctx, clientFactory, resourceType, apiVersion)
		require.Error(t, err)
		require.Nil(t, schema)
		require.Contains(t, err.Error(), "invalid-resource-type-format")
	})

	t.Run("error - API version not found", func(t *testing.T) {
		// Create a client factory that returns 404 for API version requests
		apiVersionServer := fake.APIVersionsServer{
			Get: func(ctx context.Context, planeName string, providerNamespace string, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
				errResp.SetError(fmt.Errorf("API version not found"))
				return
			},
		}

		clientFactory, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
					APIVersionsServer: apiVersionServer,
				}),
			},
		})
		require.NoError(t, err)

		ctx := context.Background()
		resourceType := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/test-resource"
		apiVersion := "nonexistent-version"

		schema, err := GetSchemaForResourceType(ctx, clientFactory, resourceType, apiVersion)
		require.Error(t, err)
		require.Nil(t, schema)
		require.ErrorIs(t, err, ErrNoSchemaFound)
	})

	t.Run("error - no schema in response", func(t *testing.T) {
		// Create a client factory that returns empty schema
		apiVersionServer := fake.APIVersionsServer{
			Get: func(ctx context.Context, planeName string, providerNamespace string, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
				response := v20231001preview.APIVersionsClientGetResponse{
					APIVersionResource: v20231001preview.APIVersionResource{
						Properties: &v20231001preview.APIVersionProperties{
							Schema: nil, // No schema
						},
					},
				}
				resp.SetResponse(http.StatusOK, response, nil)
				return
			},
		}

		clientFactory, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
			ClientOptions: policy.ClientOptions{
				Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
					APIVersionsServer: apiVersionServer,
				}),
			},
		})
		require.NoError(t, err)

		ctx := context.Background()
		resourceType := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/test-resource"
		apiVersion := "2023-10-01-preview"

		schema, err := GetSchemaForResourceType(ctx, clientFactory, resourceType, apiVersion)
		require.Error(t, err)
		require.Nil(t, schema)
		require.ErrorIs(t, err, ErrNoSchemaFound)
	})

	t.Run("success - different resource types", func(t *testing.T) {
		clientFactory, err := testUCPClientFactory()
		require.NoError(t, err)

		ctx := context.Background()

		testCases := []struct {
			name         string
			resourceType string
			apiVersion   string
		}{
			{
				name:         "containers",
				resourceType: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/test-resource",
				apiVersion:   "2023-10-01-preview",
			},
			{
				name:         "environments",
				resourceType: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env",
				apiVersion:   "2023-10-01-preview",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				schema, err := GetSchemaForResourceType(ctx, clientFactory, tc.resourceType, tc.apiVersion)
				require.NoError(t, err)
				require.NotNil(t, schema)
			})
		}
	})
}
