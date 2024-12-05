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

package resourcegroups

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/pkg/ucp/store"
)

// NotFoundError is returned when a resource group or plane is not found.
type NotFoundError struct {
	Message string
}

// Error returns the error message.
func (e *NotFoundError) Error() string {
	return e.Message
}

// Is returns true if the error is a NotFoundError.
func (e *NotFoundError) Is(err error) bool {
	_, ok := err.(*NotFoundError)
	return ok
}

// InvalidError is returned when the data is invalid.
type InvalidError struct {
	Message string
}

// Error returns the error message.
func (e *InvalidError) Error() string {
	return e.Message
}

// Is returns true if the error is a InvalidError.
func (e *InvalidError) Is(err error) bool {
	_, ok := err.(*InvalidError)
	return ok
}

// ValidateRadiusPlane validates that the plane specified in the id exists. Returns NotFoundError if the plane does not exist.
func ValidateRadiusPlane(ctx context.Context, client store.StorageClient, id resources.ID) (*datamodel.RadiusPlane, error) {
	planeID, err := resources.ParseScope(id.PlaneScope())
	if err != nil {
		// Not expected to happen.
		return nil, err
	}

	plane, err := store.GetResource[datamodel.RadiusPlane](ctx, client, planeID.String())
	if errors.Is(err, &store.ErrNotFound{}) {
		return nil, &NotFoundError{Message: fmt.Sprintf("plane %q not found", planeID.String())}
	} else if err != nil {
		return nil, fmt.Errorf("failed to fetch plane %q: %w", planeID.String(), err)
	}

	return plane, nil
}

// ValidateResourceGroup validates that the resource group specified in the id exists (if applicable).
// Returns NotFoundError if the resource group does not exist.
func ValidateResourceGroup(ctx context.Context, client store.StorageClient, id resources.ID) error {
	// If the ID contains a resource group, validate it now.
	if id.FindScope(resources_radius.ScopeResourceGroups) == "" {
		return nil
	}

	resourceGroupID, err := resources.ParseScope(id.RootScope())
	if err != nil {
		// Not expected to happen.
		return err
	}

	_, err = store.GetResource[datamodel.ResourceGroup](ctx, client, resourceGroupID.String())
	if errors.Is(err, &store.ErrNotFound{}) {
		return &NotFoundError{Message: fmt.Sprintf("resource group %q not found", resourceGroupID.String())}
	} else if err != nil {
		return fmt.Errorf("failed to fetch resource group %q: %w", resourceGroupID.String(), err)
	}

	return nil
}

// ValidateResourceType performs semantic validation of a proxy request against registered
// resource types.
//
// Returns NotFoundError if the resource type does not exist.
// Returns InvalidError if the request cannot be routed due to an invalid configuration.
func ValidateResourceType(ctx context.Context, client store.StorageClient, id resources.ID, locationName string, apiVersion string) (*url.URL, error) {
	// The strategy is to:
	// - Look up the resource type and validate that it exists .. then
	// - Look up the location resource, and validate that it supports the requested resource type and API version.

	// We need to do both because they may not be in sync. This can be be the case if a resource type is being added or deleted.

	if !isOperationResourceType(id) {
		resourceTypeID, err := datamodel.ResourceTypeIDFromResourceID(id)
		if err != nil {
			return nil, err
		}

		_, err = store.GetResource[datamodel.ResourceType](ctx, client, resourceTypeID.String())
		if errors.Is(err, &store.ErrNotFound{}) {

			// Return the error as-is to fallback to the legacy routing behavior.
			return nil, err

			// Uncomment this when we remove the legacy routing behavior.
			// return nil, &InvalidError{Message: fmt.Sprintf("resource type %q not found", id.Type())}
		} else if err != nil {
			return nil, fmt.Errorf("failed to fetch resource type %q: %w", id.Type(), err)
		}
	}

	locationID, err := datamodel.ResourceProviderLocationIDFromResourceID(id, locationName)
	if err != nil {
		return nil, err
	}

	location, err := store.GetResource[datamodel.Location](ctx, client, locationID.String())
	if errors.Is(err, &store.ErrNotFound{}) {

		// Return the error as-is to fallback to the legacy routing behavior.
		return nil, err

		// Uncomment this when we remove the legacy routing behavior.
		// return nil, &InvalidError{Message: fmt.Sprintf("location %q not found for resource provider %q", locationName, id.ProviderNamespace())}
	} else if err != nil {
		return nil, fmt.Errorf("failed to fetch location %q: %w", locationID.String(), err)
	}

	// Check if the location supports the resource type.
	// Resource types are case-insensitive so we have to iterate.
	var locationResourceType *datamodel.LocationResourceTypeConfiguration

	// We special-case two pseudo-resource types: "locations/operationstatuses" and "locations/operationresults".
	// If the resource type is one of these, we can return the downstream URL directly.
	if isOperationResourceType(id) {
		locationResourceType = &datamodel.LocationResourceTypeConfiguration{
			APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{
				apiVersion: {}, // Assume this API version is supported.
			},
		}
	} else {
		// Ex: Applications.Test/testResources => testResources
		search := strings.TrimPrefix(strings.ToLower(id.Type()), strings.ToLower(id.ProviderNamespace())+resources.SegmentSeparator)
		for name, rt := range location.Properties.ResourceTypes {
			if strings.EqualFold(name, search) {
				copy := rt
				locationResourceType = &copy
				break
			}
		}
	}

	// Now check if the location supports the resource type and API version. If it does, we can return the downstream URL.
	if locationResourceType == nil {
		return nil, &InvalidError{Message: fmt.Sprintf("resource type %q not supported by location %q", id.Type(), locationName)}
	}

	_, ok := locationResourceType.APIVersions[apiVersion]
	if !ok {
		return nil, &InvalidError{Message: fmt.Sprintf("api version %q is not supported for resource type %q by location %q", apiVersion, id.Type(), locationName)}
	}

	// If we get to here, then we're all good.
	//
	// The address might be nil which means that we're using the default address (dynamic RP)
	if location.Properties.Address == nil {
		return nil, nil
	}

	// If the address was provided, then use that instead.
	u, err := url.Parse(*location.Properties.Address)
	if err != nil {
		return nil, &InvalidError{Message: fmt.Sprintf("failed to parse location address: %v", err.Error())}
	}

	return u, nil
}

// isOperationResourceType returns true if the resource type is an operation resource type (operationResults/operationStatuses).
//
// We special-case these types, and don't require the resource provider to register them.
func isOperationResourceType(id resources.ID) bool {
	// For a resource provider "Applications.Test" the operation resource types are:
	// - Applications.Test/locations/operationStatuses
	// - Applications.Test/locations/operationResults

	// Radius resource providers include the location name in the resource id
	if strings.EqualFold(id.Type(), id.ProviderNamespace()+"/locations/operationstatuses") ||
		strings.EqualFold(id.Type(), id.ProviderNamespace()+"/locations/operationresults") {
		return true
	}

	// An older pattern is to use a child resource
	typeSegments := id.TypeSegments()
	if len(typeSegments) >= 2 && (strings.EqualFold(typeSegments[len(typeSegments)-1].Type, "operationstatuses") ||
		strings.EqualFold(typeSegments[len(typeSegments)-1].Type, "operationresults")) {
		return true
	}

	// Not an operation.
	return false
}

// ValidateLegacyResourceProvider validates that the resource provider specified in the id exists. Returns InvalidError if the plane
// contains invalid data.
func ValidateLegacyResourceProvider(ctx context.Context, client store.StorageClient, id resources.ID, plane *datamodel.RadiusPlane) (*url.URL, error) {
	downstream := plane.LookupResourceProvider(id.ProviderNamespace())
	if downstream == "" {
		return nil, &InvalidError{Message: fmt.Sprintf("resource provider %s not configured", id.ProviderNamespace())}
	}

	downstreamURL, err := url.Parse(downstream)
	if err != nil {
		return nil, &InvalidError{Message: fmt.Sprintf("failed to parse downstream URL: %v", err.Error())}
	}

	return downstreamURL, nil
}

// ValidateDownstream can be used to find and validate the downstream URL for a resource.
// Returns NotFoundError for the case where the plane or resource group does not exist.
// Returns InvalidError for cases where the data is invalid, like when the resource provider is not configured.
func ValidateDownstream(ctx context.Context, client store.StorageClient, id resources.ID, location string, apiVersion string) (*url.URL, error) {
	// There are a few steps to validation:
	//
	// - The plane exists
	// - The resource group exists
	// - The resource provider is configured .. either:
	// 		- As part of the plane (legacy routing)
	// 		- As part of a resource provider resource (System.Resources/resourceProviders) (new/UDT routing)
	//

	// The plane exists.
	plane, err := ValidateRadiusPlane(ctx, client, id)
	if err != nil {
		return nil, err
	}

	// The resource group exists (if applicable).
	err = ValidateResourceGroup(ctx, client, id)
	if err != nil {
		return nil, err
	}

	// If this returns success, it means the resource type is configured using new/UDT routing.
	downstreamURL, err := ValidateResourceType(ctx, client, id, location, apiVersion)
	if errors.Is(err, &store.ErrNotFound{}) {
		// If the resource provider is not found, treat it like a legacy provider.
		return ValidateLegacyResourceProvider(ctx, client, id, plane)
	} else if err != nil {
		return nil, err
	}

	return downstreamURL, nil
}
