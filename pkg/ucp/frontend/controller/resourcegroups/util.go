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

	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/pkg/ucp/rest"
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

// ValidateDownstream can be used to find and validate the downstream URL for a resource.
// Returns NotFoundError for the case where the plane or resource group does not exist.
// Returns InvalidError for cases where the data is invalid, like when the resource provider is not configured.
func ValidateDownstream(ctx context.Context, client store.StorageClient, id *resources.ID) (*url.URL, error) {
	planeID, err := resources.ParseScope(id.PlaneScope())
	if err != nil {
		// Not expected to happen.
		return nil, err
	}

	plane, err := store.GetResource[datamodel.Plane](ctx, client, planeID.String())
	if errors.Is(err, &store.ErrNotFound{}) {
		return nil, &NotFoundError{Message: fmt.Sprintf("plane %q not found", planeID.String())}
	} else if err != nil {
		return nil, fmt.Errorf("failed to find plane %q: %w", planeID.String(), err)
	}

	if plane.Properties.Kind != rest.PlaneKindUCPNative {
		return nil, &InvalidError{Message: fmt.Sprintf("unexpected plane type %s", plane.Properties.Kind)}
	}

	// If the ID contains a resource group, validate it now.
	if id.FindScope(resources_radius.ScopeResourceGroups) != "" {
		resourceGroupID, err := resources.ParseScope(id.RootScope())
		if err != nil {
			// Not expected to happen.
			return nil, err
		}

		_, err = store.GetResource[datamodel.ResourceGroup](ctx, client, resourceGroupID.String())
		if errors.Is(err, &store.ErrNotFound{}) {
			return nil, &NotFoundError{Message: fmt.Sprintf("resource group %q not found", resourceGroupID.String())}
		} else if err != nil {
			return nil, fmt.Errorf("failed to find resource group %q: %w", resourceGroupID.String(), err)
		}
	}

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
