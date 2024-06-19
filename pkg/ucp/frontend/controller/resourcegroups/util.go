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

// RoutingType specifies the type of routing to apply to the request.
type RoutingType string

const (
	// RoutingTypeInvalid is used when the routing type cannot be determined due to an error.
	RoutingTypeInvalid RoutingType = "invalid"

	// RoutingTypeProxy is used when the request should be proxied to the downstream URL. This
	// is used for services that implement the resource provider interface.
	RoutingTypeProxy RoutingType = "proxy"

	// RoutingTypeInternal is used when the request should be handled internally by the UCP. This
	// is used for user-defined-types.
	RoutingTypeInternal RoutingType = "internal"
)

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
		return nil, fmt.Errorf("failed to find plane %q: %w", planeID.String(), err)
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
		return fmt.Errorf("failed to find resource group %q: %w", resourceGroupID.String(), err)
	}

	return nil
}

// ValidateResourceProvider validates that the resource provider specified in the id exists (if applicable).
// Returns NotFoundError if the resource provider does not exist.
func ValidateResourceProvider(ctx context.Context, client store.StorageClient, id resources.ID) (*datamodel.ResourceProvider, error) {
	providerId := makeResourceProviderID(id)
	obj, err := client.Get(ctx, providerId.String())
	if errors.Is(err, &store.ErrNotFound{}) {
		return nil, &NotFoundError{Message: fmt.Sprintf("resource provider %q not found", providerId.String())}
	} else if err != nil {
		return nil, err
	}

	provider := &datamodel.ResourceProvider{}
	err = obj.As(provider)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

func makeResourceProviderID(id resources.ID) resources.ID {
	return resources.MustParse(resources.MakeUCPID(
		// /planes/radius/{planeName}
		id.ScopeSegments()[0:1],

		// /providers/
		[]resources.TypeSegment{
			{
				Type: datamodel.ResourceProviderResourceType,
				Name: id.ProviderNamespace(),
			},
		},
		nil))
}

// ValidateResourceType validates that the resource type specified in the id exists.
// Returns NotFoundError if the resource type does not exist.
// Returns InvalidError if the data is invalid or the resource type is not supported at the provided location.
//
// This function does not validate the API version. API version validation is handled by the dynamic RP.
func ValidateResourceType(id resources.ID, location string, provider *datamodel.ResourceProvider) (*url.URL, RoutingType, error) {
	// First let's validate that the resource type exists.
	found := false
	for _, resourceType := range provider.Properties.ResourceTypes {
		// Look for matching resource type
		if strings.EqualFold(id.Type(), provider.Name+"/"+resourceType.ResourceType) {
			found = true
			break
		}

		// Support special cases for built-in operation types. We don't require the RP to register these with
		// UCP.
		if strings.EqualFold(id.Type(), provider.Name+"/locations/operationStatuses") {
			found = true
			break
		}
		if strings.EqualFold(id.Type(), provider.Name+"/locations/operationResults") {
			found = true
			break
		}
	}

	if !found {
		return nil, RoutingTypeInvalid, &NotFoundError{Message: fmt.Sprintf("resource type %q not found", id.Type())}
	}

	// Look for matching location
	for name, loc := range provider.Properties.Locations {
		if !strings.EqualFold(name, location) {
			continue
		}

		if strings.EqualFold(loc.Address, "internal") {
			return nil, RoutingTypeInternal, nil
		}

		downstreamURL, err := url.Parse(loc.Address)
		if err != nil {
			return nil, RoutingTypeInvalid, &InvalidError{Message: fmt.Sprintf("failed to parse downstream URL: %v", err.Error())}
		}

		return downstreamURL, RoutingTypeProxy, nil
	}

	// If we get here, the specific location is not supported.
	return nil, RoutingTypeInvalid, &InvalidError{Message: fmt.Sprintf("resource type %q not supported at location %q", id.Type(), location)}
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
func ValidateDownstream(ctx context.Context, client store.StorageClient, id resources.ID, location string) (*url.URL, RoutingType, error) {
	// There are a few steps to validation:
	//
	// - The plane exists
	// - The resource group exists
	// - The resource provider is configured
	// 		- As part of the plane (proxy routing)
	// 		- As part of a resource provider manifest (internal or proxy routing)
	//

	// The plane exists.
	plane, err := ValidateRadiusPlane(ctx, client, id)
	if err != nil {
		return nil, RoutingTypeInvalid, err
	}

	// The resource group exists (if applicable).
	err = ValidateResourceGroup(ctx, client, id)
	if err != nil {
		return nil, RoutingTypeInvalid, err
	}

	provider, err := ValidateResourceProvider(ctx, client, id)
	if errors.Is(err, &NotFoundError{}) {
		// If the resource provider is not found, check if it is a legacy provider.
		downstreamURL, err := ValidateLegacyResourceProvider(ctx, client, id, plane)
		if err != nil {
			return nil, RoutingTypeInvalid, err
		}

		return downstreamURL, RoutingTypeProxy, nil
	} else if err != nil {
		return nil, RoutingTypeInvalid, err
	}

	return ValidateResourceType(id, location, provider)
}
