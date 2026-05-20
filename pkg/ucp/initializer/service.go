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
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/hosting"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// Service implements the hosting.Service interface for registering manifests.
type Service struct {
	options *ucp.Options
}

var _ hosting.Service = (*Service)(nil)

// NewService creates a server to register manifests.
func NewService(options *ucp.Options) *Service {
	return &Service{
		options: options,
	}
}

// Name gets this service name.
func (s *Service) Name() string {
	return "initializer"
}

func (w *Service) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	manifestDir := w.options.Config.Initialization.ManifestDirectory
	if manifestDir == "" {
		logger.Info("No manifest directory specified, initialization is complete")
		return nil
	}

	if _, err := os.Stat(manifestDir); os.IsNotExist(err) {
		return fmt.Errorf("manifest directory does not exist: %w", err)
	} else if err != nil {
		return fmt.Errorf("error checking manifest directory: %w", err)
	}

	dbClient, err := w.options.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get database client: %w", err)
	}

	files, err := os.ReadDir(manifestDir)
	if err != nil {
		return fmt.Errorf("failed to read manifest directory: %w", err)
	}

	// Read and validate all manifest files, then merge by namespace so that
	// multiple per-type files sharing a namespace (e.g., containers.yaml and
	// persistentVolumes.yaml both under Radius.Compute) are registered as a
	// single resource provider with all types. This makes the on-disk manifest
	// directory the source of truth for the database.
	merged := map[string]*manifest.ResourceProvider{}
	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue
		}

		filePath := filepath.Join(manifestDir, fileInfo.Name())
		logger.Info("Loading manifest", "file", filePath)

		rp, err := manifest.ValidateManifest(ctx, filePath)
		if err != nil {
			return fmt.Errorf("failed to validate manifest %s: %w", filePath, err)
		}

		existing, ok := merged[rp.Namespace]
		if !ok {
			merged[rp.Namespace] = rp
			continue
		}

		// Merge types from this file into the existing provider for this
		// namespace. Error if a type appears in multiple files.
		for typeName := range rp.Types {
			if _, exists := existing.Types[typeName]; exists {
				return fmt.Errorf("duplicate resource type %s/%s found in multiple manifest files", rp.Namespace, typeName)
			}
		}
		for typeName, resourceType := range rp.Types {
			existing.Types[typeName] = resourceType
		}

		// Preserve location from whichever file specifies one. If multiple
		// files set a location, the later file wins.
		if len(rp.Location) > 0 {
			existing.Location = rp.Location
		}
	}

	// Register each merged provider.
	for _, rp := range merged {
		logger.Info("Registering manifest", "namespace", rp.Namespace, "types", len(rp.Types))
		if err := hydrateBuiltInResourceProviderMetadata(rp); err != nil {
			return fmt.Errorf("failed to hydrate built-in resource provider metadata for namespace %s: %w", rp.Namespace, err)
		}

		if err := registerResourceProviderDirect(ctx, dbClient, "local", *rp); err != nil {
			return fmt.Errorf("failed to register manifest for namespace %s: %w", rp.Namespace, err)
		}
	}

	logger.Info("Successfully registered manifests", "directory", manifestDir)
	return nil
}

// registerResourceProviderDirect writes resource provider metadata directly to the database,
// bypassing the HTTP API and async operation queue. This is used during server initialization
// where the resources are known to not exist yet.
func registerResourceProviderDirect(ctx context.Context, dbClient database.Client, planeName string, rp manifest.ResourceProvider) error {
	rootScope := "/planes/radius/" + planeName

	// Determine location name and address
	locationName := v1.LocationGlobal
	var address string
	for name, addr := range rp.Location {
		locationName = name
		address = addr
		break
	}

	rpID := rootScope + "/providers/System.Resources/resourceProviders/" + rp.Namespace

	// 1. Save ResourceProvider
	rpModel := &datamodel.ResourceProvider{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       rpID,
				Name:     rp.Namespace,
				Type:     datamodel.ResourceProviderResourceType,
				Location: locationName,
			},
			InternalMetadata: v1.InternalMetadata{
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
		},
	}

	if err := saveResource(ctx, dbClient, rpID, rpModel); err != nil {
		return fmt.Errorf("failed to save resource provider %s: %w", rp.Namespace, err)
	}

	// Build summary while iterating
	summaryResourceTypes := map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{}

	// 2. Save ResourceTypes and APIVersions
	for typeName, resourceType := range rp.Types {
		typeID := rpID + "/resourceTypes/" + typeName

		typeModel := &datamodel.ResourceType{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   typeID,
					Name: typeName,
					Type: datamodel.ResourceTypeResourceType,
				},
				InternalMetadata: v1.InternalMetadata{
					AsyncProvisioningState: v1.ProvisioningStateSucceeded,
				},
			},
			Properties: datamodel.ResourceTypeProperties{
				Capabilities:      resourceType.Capabilities,
				DefaultAPIVersion: resourceType.DefaultAPIVersion,
				Description:       resourceType.Description,
			},
		}

		if err := saveResource(ctx, dbClient, typeID, typeModel); err != nil {
			return fmt.Errorf("failed to save resource type %s/%s: %w", rp.Namespace, typeName, err)
		}

		summaryAPIVersions := map[string]datamodel.ResourceProviderSummaryPropertiesAPIVersion{}

		for apiVersionName, apiVersion := range resourceType.APIVersions {
			avID := typeID + "/apiVersions/" + apiVersionName

			schema, _ := apiVersion.Schema.(map[string]any)
			avModel := &datamodel.APIVersion{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   avID,
						Name: apiVersionName,
						Type: datamodel.APIVersionResourceType,
					},
					InternalMetadata: v1.InternalMetadata{
						AsyncProvisioningState: v1.ProvisioningStateSucceeded,
					},
				},
				Properties: datamodel.APIVersionProperties{
					Schema: schema,
				},
			}

			if err := saveResource(ctx, dbClient, avID, avModel); err != nil {
				return fmt.Errorf("failed to save API version %s/%s@%s: %w", rp.Namespace, typeName, apiVersionName, err)
			}

			summaryAPIVersions[apiVersionName] = datamodel.ResourceProviderSummaryPropertiesAPIVersion{
				Schema: schema,
			}
		}

		summaryResourceTypes[typeName] = datamodel.ResourceProviderSummaryPropertiesResourceType{
			DefaultAPIVersion: resourceType.DefaultAPIVersion,
			Capabilities:      resourceType.Capabilities,
			Description:       resourceType.Description,
			APIVersions:       summaryAPIVersions,
		}
	}

	// 3. Save Location
	locationID := rpID + "/locations/" + locationName
	locationResourceTypes := map[string]datamodel.LocationResourceTypeConfiguration{}
	for typeName, resourceType := range rp.Types {
		apiVersions := map[string]datamodel.LocationAPIVersionConfiguration{}
		for apiVersionName := range resourceType.APIVersions {
			apiVersions[apiVersionName] = datamodel.LocationAPIVersionConfiguration{}
		}
		locationResourceTypes[typeName] = datamodel.LocationResourceTypeConfiguration{
			APIVersions: apiVersions,
		}
	}

	locationModel := &datamodel.Location{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   locationID,
				Name: locationName,
				Type: datamodel.LocationResourceType,
			},
			InternalMetadata: v1.InternalMetadata{
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
		},
		Properties: datamodel.LocationProperties{
			ResourceTypes: locationResourceTypes,
		},
	}
	if address != "" {
		locationModel.Properties.Address = &address
	}

	if err := saveResource(ctx, dbClient, locationID, locationModel); err != nil {
		return fmt.Errorf("failed to save location %s/%s: %w", rp.Namespace, locationName, err)
	}

	// 4. Save ResourceProviderSummary
	summaryID := rootScope + "/providers/System.Resources/resourceProviderSummaries/" + rp.Namespace
	summaryModel := &datamodel.ResourceProviderSummary{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   summaryID,
				Name: rp.Namespace,
				Type: datamodel.ResourceProviderSummaryResourceType,
			},
		},
		Properties: datamodel.ResourceProviderSummaryProperties{
			Locations: map[string]datamodel.ResourceProviderSummaryPropertiesLocation{
				locationName: {},
			},
			ResourceTypes: summaryResourceTypes,
		},
	}

	if err := saveResource(ctx, dbClient, summaryID, summaryModel); err != nil {
		return fmt.Errorf("failed to save resource provider summary %s: %w", rp.Namespace, err)
	}

	return nil
}

func saveResource(ctx context.Context, dbClient database.Client, id string, data any) error {
	return dbClient.Save(ctx, &database.Object{
		Metadata: database.Metadata{ID: id},
		Data:     data,
	})
}
