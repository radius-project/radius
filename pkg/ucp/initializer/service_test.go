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

package initializer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_registerResourceProviderDirect(t *testing.T) {
	t.Parallel()

	t.Run("registers all resources for a simple manifest", func(t *testing.T) {
		t.Parallel()
		dbClient := inmemory.NewClient()

		rp := createTestResourceProvider()
		err := registerResourceProviderDirect(context.Background(), dbClient, "local", rp)
		require.NoError(t, err)

		// Verify resource provider was saved
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/MyCompany.Resources")
		require.NoError(t, err)

		rpModel := &datamodel.ResourceProvider{}
		require.NoError(t, obj.As(rpModel))
		assert.Equal(t, "MyCompany.Resources", rpModel.Name)
		assert.Equal(t, datamodel.ResourceProviderResourceType, rpModel.Type)
		assert.Equal(t, v1.ProvisioningStateSucceeded, rpModel.InternalMetadata.AsyncProvisioningState)
		assert.Equal(t, "global", rpModel.Location)

		// Verify resource type was saved
		obj, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/MyCompany.Resources/resourceTypes/widgets")
		require.NoError(t, err)

		rtModel := &datamodel.ResourceType{}
		require.NoError(t, obj.As(rtModel))
		assert.Equal(t, "widgets", rtModel.Name)
		assert.Equal(t, datamodel.ResourceTypeResourceType, rtModel.Type)
		assert.Equal(t, v1.ProvisioningStateSucceeded, rtModel.InternalMetadata.AsyncProvisioningState)

		// Verify API version was saved
		obj, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/MyCompany.Resources/resourceTypes/widgets/apiVersions/2023-10-01-preview")
		require.NoError(t, err)

		avModel := &datamodel.APIVersion{}
		require.NoError(t, obj.As(avModel))
		assert.Equal(t, "2023-10-01-preview", avModel.Name)
		assert.Equal(t, datamodel.APIVersionResourceType, avModel.Type)

		// Verify location was saved
		obj, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/MyCompany.Resources/locations/global")
		require.NoError(t, err)

		locModel := &datamodel.Location{}
		require.NoError(t, obj.As(locModel))
		assert.Equal(t, "global", locModel.Name)
		assert.Equal(t, "http://localhost:8080", *locModel.Properties.Address)
		assert.Contains(t, locModel.Properties.ResourceTypes, "widgets")
		assert.Contains(t, locModel.Properties.ResourceTypes["widgets"].APIVersions, "2023-10-01-preview")

		// Verify summary was saved
		obj, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviderSummaries/MyCompany.Resources")
		require.NoError(t, err)

		summaryModel := &datamodel.ResourceProviderSummary{}
		require.NoError(t, obj.As(summaryModel))
		assert.Contains(t, summaryModel.Properties.Locations, "global")
		assert.Contains(t, summaryModel.Properties.ResourceTypes, "widgets")
		assert.Contains(t, summaryModel.Properties.ResourceTypes["widgets"].APIVersions, "2023-10-01-preview")
	})

	t.Run("registers provider with multiple types and versions", func(t *testing.T) {
		t.Parallel()
		dbClient := inmemory.NewClient()

		rp := createTestResourceProviderMultiType()
		err := registerResourceProviderDirect(context.Background(), dbClient, "local", rp)
		require.NoError(t, err)

		// Verify both resource types exist
		for _, typeName := range []string{"typeA", "typeB"} {
			_, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Multi.Provider/resourceTypes/"+typeName)
			require.NoError(t, err, "expected resource type %s to exist", typeName)
		}

		// Verify typeA has two API versions
		for _, version := range []string{"2023-01-01", "2024-01-01"} {
			_, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Multi.Provider/resourceTypes/typeA/apiVersions/"+version)
			require.NoError(t, err, "expected API version %s to exist for typeA", version)
		}

		// Verify summary has both types
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviderSummaries/Multi.Provider")
		require.NoError(t, err)

		summaryModel := &datamodel.ResourceProviderSummary{}
		require.NoError(t, obj.As(summaryModel))
		assert.Len(t, summaryModel.Properties.ResourceTypes, 2)
		assert.Len(t, summaryModel.Properties.ResourceTypes["typeA"].APIVersions, 2)
		assert.Len(t, summaryModel.Properties.ResourceTypes["typeB"].APIVersions, 1)
	})

	t.Run("registers provider with no location defaults to global", func(t *testing.T) {
		t.Parallel()
		dbClient := inmemory.NewClient()

		rp := createTestResourceProvider()
		rp.Location = nil
		err := registerResourceProviderDirect(context.Background(), dbClient, "local", rp)
		require.NoError(t, err)

		// Location should default to "global"
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/MyCompany.Resources/locations/global")
		require.NoError(t, err)

		locModel := &datamodel.Location{}
		require.NoError(t, obj.As(locModel))
		assert.Equal(t, "global", locModel.Name)
		assert.Nil(t, locModel.Properties.Address) // No address when location is nil
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()
		dbClient := inmemory.NewClient()

		rp := createTestResourceProvider()

		// Register twice
		err := registerResourceProviderDirect(context.Background(), dbClient, "local", rp)
		require.NoError(t, err)

		err = registerResourceProviderDirect(context.Background(), dbClient, "local", rp)
		require.NoError(t, err)

		// Should still be readable
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/MyCompany.Resources")
		require.NoError(t, err)
		require.NotNil(t, obj)
	})

}

func Test_Run(t *testing.T) {
	t.Parallel()

	t.Run("no manifest directory skips initialization", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("")
		err := svc.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("missing manifest directory returns error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("/nonexistent/path")
		err := svc.Run(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest directory does not exist")
	})

	t.Run("registers manifests from directory", func(t *testing.T) {
		t.Parallel()

		// Create a temp directory with a test manifest
		tempDir := t.TempDir()
		manifestContent := `
namespace: Test.Provider
location:
  global: "http://localhost:9090"
types:
  myType:
    apiVersions:
      "2025-01-01":
        schema: {}
`
		err := os.WriteFile(filepath.Join(tempDir, "test.yaml"), []byte(manifestContent), 0600)
		require.NoError(t, err)

		svc := newTestService(tempDir)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		err = svc.Run(context.Background())
		require.NoError(t, err)

		// Verify the resource provider was registered
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Test.Provider")
		require.NoError(t, err)
		require.NotNil(t, obj)
	})

	t.Run("errors on duplicate type across manifest files", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()

		// Two manifest files with the same namespace and same type name.
		manifest1 := `
namespace: Radius.Compute
types:
  containers:
    apiVersions:
      "2025-08-01-preview":
        schema: {}
`
		manifest2 := `
namespace: Radius.Compute
types:
  containers:
    apiVersions:
      "2025-08-01-preview":
        schema: {}
`
		err := os.WriteFile(filepath.Join(tempDir, "a-containers.yaml"), []byte(manifest1), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "b-containers.yaml"), []byte(manifest2), 0600)
		require.NoError(t, err)

		svc := newTestService(tempDir)
		err = svc.Run(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate resource type Radius.Compute/containers")
	})

	t.Run("merges types from multiple files sharing a namespace", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()

		manifest1 := `
namespace: Radius.Compute
types:
  containers:
    apiVersions:
      "2025-08-01-preview":
        schema: {}
`
		manifest2 := `
namespace: Radius.Compute
types:
  routes:
    apiVersions:
      "2025-08-01-preview":
        schema: {}
`
		err := os.WriteFile(filepath.Join(tempDir, "containers.yaml"), []byte(manifest1), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "routes.yaml"), []byte(manifest2), 0600)
		require.NoError(t, err)

		svc := newTestService(tempDir)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		err = svc.Run(context.Background())
		require.NoError(t, err)

		// Verify both types are registered under the same namespace.
		_, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Compute/resourceTypes/containers")
		require.NoError(t, err)
		_, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Compute/resourceTypes/routes")
		require.NoError(t, err)

		// Verify the location contains both types.
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Compute/locations/global")
		require.NoError(t, err)
		location := &datamodel.Location{}
		require.NoError(t, obj.As(location))
		assert.Contains(t, location.Properties.ResourceTypes, "containers")
		assert.Contains(t, location.Properties.ResourceTypes, "routes")
	})

	t.Run("hydrates Radius.Core schemas from embedded OpenAPI", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		manifestPath := filepath.Join("..", "..", "..", "deploy", "manifest", "built-in-providers", "dev", "radius_core.yaml")
		manifestContent, err := os.ReadFile(manifestPath)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tempDir, "radius_core.yaml"), manifestContent, 0600)
		require.NoError(t, err)

		svc := newTestService(tempDir)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		err = svc.Run(context.Background())
		require.NoError(t, err)

		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviderSummaries/Radius.Core")
		require.NoError(t, err)

		summaryModel := &datamodel.ResourceProviderSummary{}
		require.NoError(t, obj.As(summaryModel))
		require.Len(t, summaryModel.Properties.ResourceTypes, len(radiusCoreTypeOpenAPIDefinitions))

		expectedDescriptions := map[string]string{
			"applications":      "Radius Application resource",
			"bicepSettings":     "The Bicep configuration resource, providing reusable Bicep recipe settings for environments.",
			"environments":      "The environment resource",
			"recipePacks":       "The recipe pack resource",
			"terraformSettings": "The Terraform configuration resource, providing reusable Terraform recipe settings for environments.",
		}
		require.Len(t, expectedDescriptions, len(radiusCoreTypeOpenAPIDefinitions))
		for typeName, expectedDescription := range expectedDescriptions {
			resourceType := summaryModel.Properties.ResourceTypes[typeName]
			require.NotNil(t, resourceType, "resource type %q should be registered", typeName)
			require.NotNil(t, resourceType.Description, "resource type %q should have a description", typeName)
			assert.Equal(t, expectedDescription, *resourceType.Description)

			apiVersion := resourceType.APIVersions["2025-08-01-preview"]
			require.NotNil(t, apiVersion, "resource type %q should have API version 2025-08-01-preview", typeName)
			assert.Equal(t, "object", apiVersion.Schema["type"])
			requireRenderableResourceTypeSchema(t, apiVersion.Schema)
		}

		applications := summaryModel.Properties.ResourceTypes["applications"]
		applicationSchema := applications.APIVersions["2025-08-01-preview"].Schema

		applicationProperties := requireSchemaProperties(t, applicationSchema)
		environmentProperty := requireSchemaProperty(t, applicationProperties, "environment")
		assert.Equal(t, "string", environmentProperty["type"])

		statusProperty := requireSchemaProperty(t, applicationProperties, "status")
		assert.NotContains(t, statusProperty, "$ref")
		assert.Equal(t, "object", statusProperty["type"])

		environments := summaryModel.Properties.ResourceTypes["environments"]
		environmentSchema := environments.APIVersions["2025-08-01-preview"].Schema
		environmentProperties := requireSchemaProperties(t, environmentSchema)
		providersProperty := requireSchemaProperty(t, environmentProperties, "providers")
		assert.NotContains(t, providersProperty, "$ref")
		assert.Equal(t, "object", providersProperty["type"])

		recipePacks := summaryModel.Properties.ResourceTypes["recipePacks"]
		recipePackSchema := recipePacks.APIVersions["2025-08-01-preview"].Schema
		recipePackProperties := requireSchemaProperties(t, recipePackSchema)
		recipesProperty := requireSchemaProperty(t, recipePackProperties, "recipes")
		additionalProperties, ok := recipesProperty["additionalProperties"].(map[string]any)
		require.True(t, ok)
		recipeDefinitionProperties := requireSchemaProperties(t, additionalProperties)
		kindProperty := requireSchemaProperty(t, recipeDefinitionProperties, "kind")
		assert.NotContains(t, kindProperty, "$ref")
		assert.Equal(t, "string", kindProperty["type"])

		sourceProperty := requireSchemaProperty(t, recipeDefinitionProperties, "source")
		assert.NotContains(t, sourceProperty, "$ref")
		assert.Equal(t, "string", sourceProperty["type"])

		terraformSettings := summaryModel.Properties.ResourceTypes["terraformSettings"]
		terraformSettingsSchema := terraformSettings.APIVersions["2025-08-01-preview"].Schema
		terraformSettingsProperties := requireSchemaProperties(t, terraformSettingsSchema)
		terraformrcProperty := requireSchemaProperty(t, terraformSettingsProperties, "terraformrc")
		assert.NotContains(t, terraformrcProperty, "$ref")
		assert.Equal(t, "object", terraformrcProperty["type"])

		bicepSettings := summaryModel.Properties.ResourceTypes["bicepSettings"]
		bicepSettingsSchema := bicepSettings.APIVersions["2025-08-01-preview"].Schema
		bicepSettingsProperties := requireSchemaProperties(t, bicepSettingsSchema)
		registryAuthenticationsProperty := requireSchemaProperty(t, bicepSettingsProperties, "registryAuthentications")
		assert.NotContains(t, registryAuthenticationsProperty, "$ref")
		assert.Equal(t, "object", registryAuthenticationsProperty["type"])

		obj, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Core/resourceTypes/applications/apiVersions/2025-08-01-preview")
		require.NoError(t, err)

		apiVersionModel := &datamodel.APIVersion{}
		require.NoError(t, obj.As(apiVersionModel))
		assert.Equal(t, applicationSchema, apiVersionModel.Properties.Schema)
	})
}

func Test_hydrateBuiltInResourceProviderMetadata(t *testing.T) {
	t.Parallel()

	t.Run("fails when mapped Radius.Core type is missing expected API version", func(t *testing.T) {
		t.Parallel()

		rp := &manifest.ResourceProvider{
			Namespace: "Radius.Core",
			Types: map[string]*manifest.ResourceType{
				"applications": {
					APIVersions: map[string]*manifest.ResourceTypeAPIVersion{
						"2024-01-01-preview": {Schema: map[string]any{}},
					},
				},
			},
		}

		err := hydrateBuiltInResourceProviderMetadata(rp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "mapped Radius.Core type applications is missing API version 2025-08-01-preview")
	})

	t.Run("fails when Radius.Core manifest type has no OpenAPI metadata mapping", func(t *testing.T) {
		t.Parallel()

		rp := &manifest.ResourceProvider{
			Namespace: "Radius.Core",
			Types: map[string]*manifest.ResourceType{
				"widgets": {
					APIVersions: map[string]*manifest.ResourceTypeAPIVersion{
						"2025-08-01-preview": {Schema: map[string]any{}},
					},
				},
			},
		}

		err := hydrateBuiltInResourceProviderMetadata(rp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Radius.Core type widgets has no OpenAPI metadata mapping")
	})

	t.Run("ignores non Radius.Core providers", func(t *testing.T) {
		t.Parallel()

		rp := &manifest.ResourceProvider{
			Namespace: "MyCompany.Resources",
			Types: map[string]*manifest.ResourceType{
				"applications": {
					APIVersions: map[string]*manifest.ResourceTypeAPIVersion{},
				},
			},
		}

		err := hydrateBuiltInResourceProviderMetadata(rp)
		require.NoError(t, err)
	})
}

func Test_saveResource(t *testing.T) {
	t.Parallel()

	dbClient := inmemory.NewClient()
	data := &datamodel.ResourceProvider{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/planes/radius/local/providers/System.Resources/resourceProviders/Test",
				Name: "Test",
				Type: datamodel.ResourceProviderResourceType,
			},
		},
	}

	err := saveResource(context.Background(), dbClient, data.ID, data)
	require.NoError(t, err)

	obj, err := dbClient.Get(context.Background(), data.ID)
	require.NoError(t, err)

	result := &datamodel.ResourceProvider{}
	require.NoError(t, obj.As(result))
	assert.Equal(t, "Test", result.Name)
}

// newTestService creates a Service with in-memory database for testing.
func newTestService(manifestDir string) *Service {
	return &Service{
		options: &ucp.Options{
			Config: &ucp.Config{
				Initialization: ucp.InitializationConfig{
					ManifestDirectory: manifestDir,
				},
			},
			DatabaseProvider: databaseprovider.FromMemory(),
		},
	}
}

func createTestResourceProvider() manifest.ResourceProvider {
	return manifest.ResourceProvider{
		Namespace: "MyCompany.Resources",
		Location: map[string]string{
			"global": "http://localhost:8080",
		},
		Types: map[string]*manifest.ResourceType{
			"widgets": {
				APIVersions: map[string]*manifest.ResourceTypeAPIVersion{
					"2023-10-01-preview": {
						Schema: map[string]any{},
					},
				},
			},
		},
	}
}

func createTestResourceProviderMultiType() manifest.ResourceProvider {
	return manifest.ResourceProvider{
		Namespace: "Multi.Provider",
		Location: map[string]string{
			"global": "http://localhost:8080",
		},
		Types: map[string]*manifest.ResourceType{
			"typeA": {
				APIVersions: map[string]*manifest.ResourceTypeAPIVersion{
					"2023-01-01": {Schema: map[string]any{}},
					"2024-01-01": {Schema: map[string]any{}},
				},
			},
			"typeB": {
				APIVersions: map[string]*manifest.ResourceTypeAPIVersion{
					"2024-01-01": {Schema: map[string]any{}},
				},
			},
		},
	}
}

func requireSchemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()

	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, properties)
	return properties
}

func requireSchemaProperty(t *testing.T, properties map[string]any, name string) map[string]any {
	t.Helper()

	property, ok := properties[name].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, property)
	return property
}

func requireRenderableResourceTypeSchema(t *testing.T, schema map[string]any) {
	t.Helper()

	properties := requireSchemaProperties(t, schema)
	for name, property := range properties {
		propertySchema, ok := property.(map[string]any)
		require.True(t, ok, "property %q should be an object schema", name)
		require.NotContains(t, propertySchema, "$ref", "property %q should have expanded refs", name)
		require.IsType(t, "", propertySchema["type"], "property %q should have a concrete type", name)

		if propertySchema["type"] == "object" {
			requireRenderableNestedObjectSchema(t, name, propertySchema)
		}
	}
}

func requireRenderableNestedObjectSchema(t *testing.T, path string, schema map[string]any) {
	t.Helper()

	nestedSchema := schema
	if _, ok := nestedSchema["properties"].(map[string]any); !ok {
		additionalProperties, ok := schema["additionalProperties"].(map[string]any)
		if !ok {
			return
		}
		nestedSchema = additionalProperties
	}

	properties, ok := nestedSchema["properties"].(map[string]any)
	if !ok {
		return
	}
	require.NotEmpty(t, properties)
	for name, property := range properties {
		propertyPath := path + "." + name
		propertySchema, ok := property.(map[string]any)
		require.True(t, ok, "property %q should be an object schema", propertyPath)
		require.NotContains(t, propertySchema, "$ref", "property %q should have expanded refs", propertyPath)
		require.IsType(t, "", propertySchema["type"], "property %q should have a concrete type", propertyPath)

		if propertySchema["type"] == "object" {
			requireRenderableNestedObjectSchema(t, propertyPath, propertySchema)
		}
	}
}

// validIconSVG is a minimal SVG that passes datamodel.ValidateIcon; used across
// the icon-related tests below.
var validIconSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`)

// invalidIconSVG contains a <script> element, which datamodel.ValidateIcon
// rejects.
var invalidIconSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`)

// iconManifestYAML is a single-type manifest used by the icon tests. Kept
// separate from the test bodies so the tests are easy to read.
const iconManifestYAML = `
namespace: Test.Provider
location:
  global: "http://localhost:9090"
types:
  widgets:
    apiVersions:
      "2025-01-01":
        schema: {}
`

const iconMultiTypeManifestYAML = `
namespace: Test.Provider
location:
  global: "http://localhost:9090"
types:
  widgets:
    apiVersions:
      "2025-01-01":
        schema: {}
  gadgets:
    apiVersions:
      "2025-01-01":
        schema: {}
`

func Test_loadTypeIcon(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when no <typeName>.svg exists", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		icon, err := loadTypeIcon(dir, "widgets")
		require.NoError(t, err)
		assert.Nil(t, icon)
	})

	t.Run("returns bytes when <typeName>.svg exists and is valid", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "widgets.svg"), validIconSVG, 0600))

		icon, err := loadTypeIcon(dir, "widgets")
		require.NoError(t, err)
		require.NotNil(t, icon)
		assert.Equal(t, string(validIconSVG), *icon)
	})

	t.Run("returns error when <typeName>.svg fails validation", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "widgets.svg"), invalidIconSVG, 0600))

		icon, err := loadTypeIcon(dir, "widgets")
		require.Error(t, err)
		assert.Nil(t, icon)
		assert.Contains(t, err.Error(), "invalid icon file")
	})

	t.Run("only <typeName>.svg is considered, not the manifest basename", func(t *testing.T) {
		t.Parallel()

		// A file named after the manifest (types.svg) must NOT be applied to
		// individual types — the loader keys strictly on the type name.
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "types.svg"), validIconSVG, 0600))

		icon, err := loadTypeIcon(dir, "widgets")
		require.NoError(t, err)
		assert.Nil(t, icon)
	})
}

func Test_Run_Icons(t *testing.T) {
	t.Parallel()

	sum := sha256.Sum256(validIconSVG)
	expectedHashHex := hex.EncodeToString(sum[:])

	t.Run("applies <typeName>.svg to the matching type and stores server-computed hash", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "widgets.yaml"), []byte(iconManifestYAML), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "widgets.svg"), validIconSVG, 0600))

		svc := newTestService(tempDir)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		require.NoError(t, svc.Run(context.Background()))

		// Verify the resource type record carries the icon and hash.
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Test.Provider/resourceTypes/widgets")
		require.NoError(t, err)

		rt := &datamodel.ResourceType{}
		require.NoError(t, obj.As(rt))
		require.NotNil(t, rt.Properties.Icon)
		assert.Equal(t, string(validIconSVG), *rt.Properties.Icon)
		require.NotNil(t, rt.Properties.IconHash)
		assert.Equal(t, expectedHashHex, *rt.Properties.IconHash)

		// Verify the summary mirrors the same icon and hash.
		obj, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviderSummaries/Test.Provider")
		require.NoError(t, err)

		summary := &datamodel.ResourceProviderSummary{}
		require.NoError(t, obj.As(summary))
		require.Contains(t, summary.Properties.ResourceTypes, "widgets")
		summaryType := summary.Properties.ResourceTypes["widgets"]
		require.NotNil(t, summaryType.Icon)
		assert.Equal(t, string(validIconSVG), *summaryType.Icon)
		require.NotNil(t, summaryType.IconHash)
		assert.Equal(t, expectedHashHex, *summaryType.IconHash)
	})

	t.Run("in a multi-type manifest, <typeName>.svg applies only to the matching type", func(t *testing.T) {
		t.Parallel()

		// Per spec 003 FR-002 / FR-002b, a bare icon cannot silently apply to
		// every type of a multi-type manifest. The sibling-file flow mirrors
		// that: a <typeName>.svg is scoped to that one type.
		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "types.yaml"), []byte(iconMultiTypeManifestYAML), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "widgets.svg"), validIconSVG, 0600))

		svc := newTestService(tempDir)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		require.NoError(t, svc.Run(context.Background()))

		// widgets got the icon.
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Test.Provider/resourceTypes/widgets")
		require.NoError(t, err)
		widgets := &datamodel.ResourceType{}
		require.NoError(t, obj.As(widgets))
		require.NotNil(t, widgets.Properties.Icon)
		assert.Equal(t, string(validIconSVG), *widgets.Properties.Icon)
		require.NotNil(t, widgets.Properties.IconHash)
		assert.Equal(t, expectedHashHex, *widgets.Properties.IconHash)

		// gadgets did NOT get the icon.
		obj, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Test.Provider/resourceTypes/gadgets")
		require.NoError(t, err)
		gadgets := &datamodel.ResourceType{}
		require.NoError(t, obj.As(gadgets))
		assert.Nil(t, gadgets.Properties.Icon)
		assert.Nil(t, gadgets.Properties.IconHash)
	})

	t.Run("manifest without <typeName>.svg leaves icon and hash nil", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "widgets.yaml"), []byte(iconManifestYAML), 0600))

		svc := newTestService(tempDir)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		require.NoError(t, svc.Run(context.Background()))

		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Test.Provider/resourceTypes/widgets")
		require.NoError(t, err)

		rt := &datamodel.ResourceType{}
		require.NoError(t, obj.As(rt))
		assert.Nil(t, rt.Properties.Icon)
		assert.Nil(t, rt.Properties.IconHash)
	})

	t.Run("skips stray svg files during directory scan", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		// An .svg with no matching .yaml — Run must ignore it rather than try to parse it as a manifest.
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "orphan.svg"), validIconSVG, 0600))

		svc := newTestService(tempDir)
		require.NoError(t, svc.Run(context.Background()))
	})

	t.Run("invalid <typeName>.svg fails startup", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "widgets.yaml"), []byte(iconManifestYAML), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(tempDir, "widgets.svg"), invalidIconSVG, 0600))

		svc := newTestService(tempDir)
		err := svc.Run(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid icon file")
	})
}

func Test_registerResourceProviderDirect_Icon(t *testing.T) {
	t.Parallel()

	svg := "<svg xmlns=\"http://www.w3.org/2000/svg\"><rect/></svg>"
	sum := sha256.Sum256([]byte(svg))
	expectedHashHex := hex.EncodeToString(sum[:])

	dbClient := inmemory.NewClient()
	rp := createTestResourceProvider()
	rp.Types["widgets"].Icon = to.Ptr(svg)

	require.NoError(t, registerResourceProviderDirect(context.Background(), dbClient, "local", rp))

	// The resource type record carries the icon and its server-computed SHA-256 hash.
	obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/MyCompany.Resources/resourceTypes/widgets")
	require.NoError(t, err)

	rt := &datamodel.ResourceType{}
	require.NoError(t, obj.As(rt))
	require.NotNil(t, rt.Properties.Icon)
	assert.Equal(t, svg, *rt.Properties.Icon)
	require.NotNil(t, rt.Properties.IconHash)
	assert.Equal(t, expectedHashHex, *rt.Properties.IconHash)

	// The summary mirror carries the same icon and hash.
	obj, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviderSummaries/MyCompany.Resources")
	require.NoError(t, err)

	summary := &datamodel.ResourceProviderSummary{}
	require.NoError(t, obj.As(summary))
	require.Contains(t, summary.Properties.ResourceTypes, "widgets")
	summaryType := summary.Properties.ResourceTypes["widgets"]
	require.NotNil(t, summaryType.Icon)
	assert.Equal(t, svg, *summaryType.Icon)
	require.NotNil(t, summaryType.IconHash)
	assert.Equal(t, expectedHashHex, *summaryType.IconHash)
}
