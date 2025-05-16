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

package resourceproviders

import (
	"context"
	"fmt"

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ ctrl.Controller = (*APIVersionPutController)(nil)

// APIVersionPutController is the async operation controller to perform PUT operations on API versions.
type APIVersionPutController struct {
	ctrl.BaseController
}

// Run implements the controller interface.
func (c *APIVersionPutController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	id, summaryID, err := resourceProviderSummaryIDFromRequest(request)
	if err != nil {
		return ctrl.Result{}, err
	}

	apiVersion, err := c.fetchAPIVersion(ctx, id)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = updateResourceProviderSummaryWithETag(ctx, c.DatabaseClient(), summaryID, summaryNotFoundFail, c.updateSummary(id, apiVersion))
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (c *APIVersionPutController) fetchAPIVersion(ctx context.Context, id resources.ID) (*datamodel.APIVersion, error) {
	obj, err := c.DatabaseClient().Get(ctx, id.String())
	if err != nil {
		return nil, err
	}

	apiVersion := datamodel.APIVersion{}
	err = obj.As(&apiVersion)
	if err != nil {
		return nil, err
	}

	return &apiVersion, nil
}

func (c *APIVersionPutController) updateSummary(id resources.ID, apiVersion *datamodel.APIVersion) func(summary *datamodel.ResourceProviderSummary) error {
	return func(summary *datamodel.ResourceProviderSummary) error {
		if summary.Properties.ResourceTypes == nil {
			summary.Properties.ResourceTypes = map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{}
		}

		resourceTypeName := id.Truncate().Name()
		resourceTypeEntry, ok := summary.Properties.ResourceTypes[id.Truncate().Name()]
		if !ok {
			// If we get here, the resource type entry doesn't exist! Something is out of whack.
			return fmt.Errorf("resource type entry %q not found", resourceTypeName)
		}

		if resourceTypeEntry.APIVersions == nil {
			resourceTypeEntry.APIVersions = map[string]datamodel.ResourceProviderSummaryPropertiesAPIVersion{}
		}

		apiVersionName := id.Name()
		_, ok = resourceTypeEntry.APIVersions[apiVersionName]
		resourceTypeEntry.APIVersions[apiVersionName] = datamodel.ResourceProviderSummaryPropertiesAPIVersion{
			Schema: apiVersion.Properties.Schema,
		}

		summary.Properties.ResourceTypes[resourceTypeName] = resourceTypeEntry

		return nil
	}
}
