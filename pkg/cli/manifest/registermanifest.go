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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

const (
	initialBackoff = 2 * time.Second
	maxRetries     = 5
)

// RegisterFile registers a manifest file
func RegisterFile(ctx context.Context, clientFactory *v20231001preview.ClientFactory, planeName string, filePath string, logger func(format string, args ...any)) error {
	if filePath == "" {
		return fmt.Errorf("invalid manifest file path")
	}

	resourceProvider, err := ReadFile(filePath)
	if err != nil {
		return err
	}

	return RegisterResourceProvider(ctx, clientFactory, planeName, *resourceProvider, logger)
}

// RegisterResourceProvider registers a resource provider
func RegisterResourceProvider(ctx context.Context, clientFactory *v20231001preview.ClientFactory, planeName string, resourceProvider ResourceProvider, logger func(format string, args ...any)) error {
	var locationName string
	var address string

	if resourceProvider.Location == nil {
		locationName = v1.LocationGlobal
	} else {
		for locationName, address = range resourceProvider.Location {
			// We support one location per resourceProvider
			break
		}
	}

	err := retryOperation(ctx, func() error {
		resourceProviderPoller, err := clientFactory.NewResourceProvidersClient().BeginCreateOrUpdate(
			ctx, planeName, resourceProvider.Name,
			v20231001preview.ResourceProviderResource{
				Location:   to.Ptr(locationName),
				Properties: &v20231001preview.ResourceProviderProperties{},
			}, nil)
		if err != nil {
			return err
		}
		_, err = resourceProviderPoller.PollUntilDone(ctx, nil)
		if err != nil {
			return err // also retried if error indicates a 409 conflict
		}
		return nil
	}, logger)
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
		err = retryOperation(ctx, func() error {
			resourceTypePoller, err := clientFactory.NewResourceTypesClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, resourceTypeName, v20231001preview.ResourceTypeResource{
				Properties: &v20231001preview.ResourceTypeProperties{
					Capabilities:      to.SliceOfPtrs(resourceType.Capabilities...),
					DefaultAPIVersion: resourceType.DefaultAPIVersion,
					Description:       resourceType.Description,
				},
			}, nil)
			if err != nil {
				return err
			}
			_, err = resourceTypePoller.PollUntilDone(ctx, nil)
			if err != nil {
				return err
			}
			return nil
		}, logger)
		if err != nil {
			return err
		}

		locationResourceType := &v20231001preview.LocationResourceType{
			APIVersions: map[string]map[string]any{},
		}

		for apiVersionName := range resourceType.APIVersions {
			logIfEnabled(logger, "Creating API Version %s/%s@%s", resourceProvider.Name, resourceTypeName, apiVersionName)
			schema := resourceType.APIVersions[apiVersionName].Schema.(map[string]any)
			err = retryOperation(ctx, func() error {
				apiVersionsPoller, err := clientFactory.NewAPIVersionsClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, resourceTypeName, apiVersionName, v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{
						Schema: schema,
					},
				}, nil)
				if err != nil {
					return err
				}
				_, err = apiVersionsPoller.PollUntilDone(ctx, nil)
				if err != nil {
					return err
				}
				return nil
			}, logger)
			if err != nil {
				return err
			}
			locationResourceType.APIVersions[apiVersionName] = map[string]any{}
		}

		locationResource.Properties.ResourceTypes[resourceTypeName] = locationResourceType
	}

	if address != "" {
		locationResource.Properties.Address = to.Ptr(address)
	}

	logIfEnabled(logger, "Creating location %s/%s/%s", resourceProvider.Name, locationName, address)
	err = retryOperation(ctx, func() error {
		locationPoller, err := clientFactory.NewLocationsClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, locationName, locationResource, nil)
		if err != nil {
			return err
		}
		_, err = locationPoller.PollUntilDone(ctx, nil)
		if err != nil {
			return err
		}
		return nil
	}, logger)
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

	files, err := os.ReadDir(directoryPath)
	if err != nil {
		return err
	}

	for _, fileInfo := range files {
		if fileInfo.IsDir() {
			continue
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

// RegisterType registers a type specified in a manifest file
func RegisterType(ctx context.Context, clientFactory *v20231001preview.ClientFactory, planeName string, filePath string, typeName string, logger func(format string, args ...any)) error {
	if filePath == "" {
		return fmt.Errorf("invalid manifest file path")
	}

	resourceProvider, err := ReadFile(filePath)
	if err != nil {
		return err
	}

	var locationName string
	var address string

	if resourceProvider.Location == nil {
		locationName = v1.LocationGlobal
	} else {
		for locationName, address = range resourceProvider.Location {
			// We support one location per resourceProvider
			break
		}
	}

	resourceType, ok := resourceProvider.Types[typeName]
	if !ok {
		return fmt.Errorf("type %s not found in manifest file %s", typeName, filePath)
	}

	logIfEnabled(logger, "Creating resource type %s/%s with capabilities %s ", resourceProvider.Name, typeName, strings.Join(resourceType.Capabilities, ","))

	err = retryOperation(ctx, func() error {
		resourceTypePoller, err := clientFactory.NewResourceTypesClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, typeName, v20231001preview.ResourceTypeResource{
			Properties: &v20231001preview.ResourceTypeProperties{
				Capabilities:      to.SliceOfPtrs(resourceType.Capabilities...),
				DefaultAPIVersion: resourceType.DefaultAPIVersion,
				Description:       resourceType.Description,
			},
		}, nil)
		if err != nil {
			return err
		}

		_, err = resourceTypePoller.PollUntilDone(ctx, nil)
		if err != nil {
			return err
		}
		return nil
	}, logger)
	if err != nil {
		return err
	}

	for apiVersionName := range resourceType.APIVersions {
		schema := resourceType.APIVersions[apiVersionName].Schema.(map[string]any)
		logIfEnabled(logger, "Creating API Version %s/%s@%s", resourceProvider.Name, typeName, apiVersionName)
		apiVersionsPoller, err := clientFactory.NewAPIVersionsClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, typeName, apiVersionName, v20231001preview.APIVersionResource{
			Properties: &v20231001preview.APIVersionProperties{
				Schema: schema,
			},
		}, nil)
		if err != nil {
			return err
		}

		_, err = apiVersionsPoller.PollUntilDone(ctx, nil)
		if err != nil {
			return err
		}
	}

	// get the existing location resource and update it with new resource type. We have to revisit this code once schema is finalized and validated.
	locationResourceGetResponse, err := clientFactory.NewLocationsClient().Get(ctx, planeName, resourceProvider.Name, locationName, nil)
	if err != nil {
		return err
	}

	locationResource := locationResourceGetResponse.LocationResource
	if address != "" {
		locationResource.Properties.Address = to.Ptr(address)
	}

	locationResource.Properties.ResourceTypes[typeName] = &v20231001preview.LocationResourceType{
		APIVersions: map[string]map[string]any{},
	}
	for apiVersionName := range resourceType.APIVersions {
		locationResource.Properties.ResourceTypes[typeName].APIVersions[apiVersionName] = map[string]any{}
	}

	logIfEnabled(logger, "Updating location %s/%s with new resource type", resourceProvider.Name, locationName)
	locationPoller, err := clientFactory.NewLocationsClient().BeginCreateOrUpdate(ctx, planeName, resourceProvider.Name, locationName, locationResource, nil)
	if err != nil {
		return err
	}

	_, err = locationPoller.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	logIfEnabled(logger, "Resource type %s/%s created successfully", resourceProvider.Name, typeName)
	return nil
}

// Define an optional logger to prevent nil pointer dereference
func logIfEnabled(logger func(format string, args ...any), format string, args ...any) {
	if logger != nil {
		logger(format, args...)
	}
}

// retryOperation retries an operation with exponential backoff upon a 409 conflict.
// It also handles context cancellation or timeouts, returning immediately if ctx is done.
func retryOperation(ctx context.Context, operation func() error, logger func(format string, args ...any)) error {
	backoff := initialBackoff

	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = operation()
		if err != nil {
			if is409ConflictError(err) {
				if logger != nil {
					logger("Got 409 conflict on attempt %d/%d with error: %v. Retrying in %s...", attempt, maxRetries, err, backoff)
				}
				// Wait for either the context to be cancelled or the backoff duration to pass
				select {
				case <-ctx.Done():
					// Context cancelled or timed out
					return ctx.Err()
				case <-time.After(backoff):
					// Increase backoff and try again
					backoff *= 2
					continue
				}
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("exceeded %d retries, err: %w", maxRetries, err)
}

// is409ConflictError returns true if the error is a 409 Conflict error
func is409ConflictError(err error) bool {
	if err == nil {
		return false
	}

	var respErr *azcore.ResponseError
	return errors.As(err, &respErr) && respErr.StatusCode == 409
}
