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
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
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
		svc := newTestService("", nil)
		err := svc.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("missing manifest directory returns error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService("/nonexistent/path", nil)
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

		svc := newTestService(tempDir, nil)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		err = svc.Run(context.Background())
		require.NoError(t, err)

		// Verify the resource provider was registered
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Test.Provider")
		require.NoError(t, err)
		require.NotNil(t, obj)
	})

	t.Run("registers embedded manifests", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Embedded/myType
`),
			},
			"Embedded/myType/myType.yaml": &fstest.MapFile{
				Data: []byte(`
namespace: Radius.Embedded
types:
  myType:
    apiVersions:
      "2025-01-01":
        schema: {}
`),
			},
		}

		svc := newTestService("", fsys)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		err = svc.Run(context.Background())
		require.NoError(t, err)

		// Verify the embedded resource provider was registered
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Embedded")
		require.NoError(t, err)
		require.NotNil(t, obj)
	})

	t.Run("registers both embedded and directory manifests", func(t *testing.T) {
		t.Parallel()

		// Embedded manifest
		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Embedded/embeddedType
`),
			},
			"Embedded/embeddedType/embeddedType.yaml": &fstest.MapFile{
				Data: []byte(`
namespace: Radius.Embedded
types:
  embeddedType:
    apiVersions:
      "2025-01-01":
        schema: {}
`),
			},
		}

		// Directory manifest
		tempDir := t.TempDir()
		manifestContent := `
namespace: Directory.Provider
location:
  global: "http://localhost:9090"
types:
  dirType:
    apiVersions:
      "2025-01-01":
        schema: {}
`
		err := os.WriteFile(filepath.Join(tempDir, "dir.yaml"), []byte(manifestContent), 0600)
		require.NoError(t, err)

		svc := newTestService(tempDir, fsys)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		err = svc.Run(context.Background())
		require.NoError(t, err)

		// Both should be registered
		_, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Embedded")
		require.NoError(t, err)

		_, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Directory.Provider")
		require.NoError(t, err)
	})

	t.Run("directory and embedded manifests are additive for same namespace", func(t *testing.T) {
		t.Parallel()

		// Embedded manifest for Radius.Same/typeA
		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Same/typeA
`),
			},
			"Same/typeA/typeA.yaml": &fstest.MapFile{
				Data: []byte(`
namespace: Radius.Same
types:
  typeA:
    apiVersions:
      "2025-01-01":
        schema: {}
`),
			},
		}

		// Directory manifest for Radius.Same with typeB
		tempDir := t.TempDir()
		manifestContent := `
namespace: Radius.Same
location:
  global: "http://localhost:9090"
types:
  typeB:
    apiVersions:
      "2025-01-01":
        schema: {}
`
		err := os.WriteFile(filepath.Join(tempDir, "same.yaml"), []byte(manifestContent), 0600)
		require.NoError(t, err)

		svc := newTestService(tempDir, fsys)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		err = svc.Run(context.Background())
		require.NoError(t, err)

		// Individual resource type records persist (create-or-update).
		// typeA from embedded
		_, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Same/resourceTypes/typeA")
		require.NoError(t, err)

		// typeB from directory
		_, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Same/resourceTypes/typeB")
		require.NoError(t, err)

		// The provider summary reflects the last-registered set of types (directory),
		// so it only contains typeB. This is last-write-wins at the namespace level.
		obj, err := dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviderSummaries/Radius.Same")
		require.NoError(t, err)

		summaryModel := &datamodel.ResourceProviderSummary{}
		require.NoError(t, obj.As(summaryModel))
		assert.Contains(t, summaryModel.Properties.ResourceTypes, "typeB")
		assert.NotContains(t, summaryModel.Properties.ResourceTypes, "typeA")
	})

	t.Run("simulates upgrade with additional default type", func(t *testing.T) {
		t.Parallel()

		// First run (v1): register only typeA
		fsysV1 := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Upgrade/typeA
`),
			},
			"Upgrade/typeA/typeA.yaml": &fstest.MapFile{
				Data: []byte(`
namespace: Radius.Upgrade
types:
  typeA:
    apiVersions:
      "2025-01-01":
        schema: {}
`),
			},
		}

		svc := newTestService("", fsysV1)
		dbClient, err := svc.options.DatabaseProvider.GetClient(context.Background())
		require.NoError(t, err)

		err = svc.Run(context.Background())
		require.NoError(t, err)

		// Verify typeA is registered
		_, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Upgrade/resourceTypes/typeA")
		require.NoError(t, err)

		// Second run (v2 upgrade): register typeA + typeB using same database
		fsysV2 := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Upgrade/typeA
  - Radius.Upgrade/typeB
`),
			},
			"Upgrade/typeA/typeA.yaml": &fstest.MapFile{
				Data: []byte(`
namespace: Radius.Upgrade
types:
  typeA:
    apiVersions:
      "2025-01-01":
        schema: {}
`),
			},
			"Upgrade/typeB/typeB.yaml": &fstest.MapFile{
				Data: []byte(`
namespace: Radius.Upgrade
types:
  typeB:
    apiVersions:
      "2025-01-01":
        schema: {}
`),
			},
		}

		// Create a new service with the same database provider but updated embedded FS
		svcV2 := NewService(svc.options, fsysV2)
		err = svcV2.Run(context.Background())
		require.NoError(t, err)

		// Both types should be registered after upgrade
		_, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Upgrade/resourceTypes/typeA")
		require.NoError(t, err)

		_, err = dbClient.Get(context.Background(), "/planes/radius/local/providers/System.Resources/resourceProviders/Radius.Upgrade/resourceTypes/typeB")
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
func newTestService(manifestDir string, embeddedFS fs.FS) *Service {
	return NewService(&ucp.Options{
		Config: &ucp.Config{
			Initialization: ucp.InitializationConfig{
				ManifestDirectory: manifestDir,
			},
		},
		DatabaseProvider: databaseprovider.FromMemory(),
	}, embeddedFS)
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
