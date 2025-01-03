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

package manifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// RegisterFile registers a manifest file
func RegisterFile(ctx context.Context, clientFactory *v20231001preview.ClientFactory, planeName string, filePath string, logger func(format string, args ...any)) error {
	// Check for valid file path
	if filePath == "" {
		return fmt.Errorf("invalid manifest file path")
	}

	// Read the manifest file
	resourceProvider, err := ReadFile(filePath)
	if err != nil {
		return err
	}

	logIfEnabled(logger, "Creating resource provider %s", resourceProvider.Name)
	resourceProviderPoller, err := clientFactory.NewResourceProvidersClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, v20231001preview.ResourceProviderResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceProviderProperties{},
	}, nil)
	if err != nil {
		return err
	}

	_, err = resourceProviderPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	// The location resource contains references to all of the resource types and API versions that the resource provider supports.
	// We're instantiating the struct here so we can update it as we loop.
	locationResource := v20231001preview.LocationResource{
		Properties: &v20231001preview.LocationProperties{
			ResourceTypes: map[string]*v20231001preview.LocationResourceType{},
		},
	}

	for resourceTypeName, resourceType := range resourceProvider.Types {
		logIfEnabled(logger, "Creating resource type %s/%s", resourceProvider.Name, resourceTypeName)
		resourceTypePoller, err := clientFactory.NewResourceTypesClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, resourceTypeName, v20231001preview.ResourceTypeResource{
			Properties: &v20231001preview.ResourceTypeProperties{
				Capabilities:      to.SliceOfPtrs(resourceType.Capabilities...),
				DefaultAPIVersion: resourceType.DefaultAPIVersion,
			},
		}, nil)
		if err != nil {
			return err
		}

		_, err = resourceTypePoller.PollUntilDone(ctx, nil)
		if err != nil {
			return err
		}

		locationResourceType := &v20231001preview.LocationResourceType{
			APIVersions: map[string]map[string]any{},
		}

		for apiVersionName := range resourceType.APIVersions {
			logIfEnabled(logger, "Creating API Version %s/%s@%s", resourceProvider.Name, resourceTypeName, apiVersionName)
			apiVersionsPoller, err := clientFactory.NewAPIVersionsClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, resourceTypeName, apiVersionName, v20231001preview.APIVersionResource{
				Properties: &v20231001preview.APIVersionProperties{},
			}, nil)
			if err != nil {
				return err
			}

			_, err = apiVersionsPoller.PollUntilDone(ctx, nil)
			if err != nil {
				return err
			}

			locationResourceType.APIVersions[apiVersionName] = map[string]any{}
		}

		locationResource.Properties.ResourceTypes[resourceTypeName] = locationResourceType
	}

	logIfEnabled(logger, "Creating location %s/%s", resourceProvider.Name, v1.LocationGlobal)
	locationPoller, err := clientFactory.NewLocationsClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, v1.LocationGlobal, locationResource, nil)
	if err != nil {
		return err
	}

	_, err = locationPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	_, err = clientFactory.NewResourceProvidersClient().Get(ctx, planeName, resourceProvider.Name, nil)
	if err != nil {
		return err
	}

	return nil
}

// RegisterDirectory registers all manifest files in a directory
func RegisterDirectory(ctx context.Context, clientFactory *v20231001preview.ClientFactory, planeName string, directoryPath string, logger func(format string, args ...any)) error {
	// Check for valid directory path
	if directoryPath == "" {
		return fmt.Errorf("invalid manifest directory")
	}

	info, err := os.Stat(directoryPath)
	if err != nil {
		return fmt.Errorf("failed to access manifest path %s: %w", directoryPath, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("manifest path %s is not a directory", directoryPath)
	}

	// List all files in the manifestDirectory
	files, err := os.ReadDir(directoryPath)
	if err != nil {
		return err
	}

	// Iterate over each file in the directory
	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue // Skip directories
		}
		filePath := filepath.Join(directoryPath, fileInfo.Name())

		logIfEnabled(logger, "Registering manifest %s", filePath)
		err = RegisterFile(ctx, clientFactory, planeName, filePath, logger)
		if err != nil {
			return fmt.Errorf("failed to register manifest file %s: %w", filePath, err)
		}
	}

	return nil
}

// Define an optional logger to prevent nil pointer dereference
func logIfEnabled(logger func(format string, args ...any), format string, args ...any) {
	if logger != nil {
		logger(format, args...)
	}
}
