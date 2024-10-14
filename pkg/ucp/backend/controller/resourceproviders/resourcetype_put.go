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

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ ctrl.Controller = (*ResourceTypePutController)(nil)

// ResourceTypePutController is the async operation controller to perform PUT operations on resource types.
type ResourceTypePutController struct {
	ctrl.BaseController
}

// Run implements the controller interface.
func (c *ResourceTypePutController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	id, summaryID, err := resourceProviderSummaryIDFromRequest(request)
	if err != nil {
		return ctrl.Result{}, err
	}

	resourceType, err := c.fetchResourceType(ctx, id)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = updateResourceProviderSummaryWithETag(ctx, c.StorageClient(), summaryID, summaryNotFoundFail, c.updateSummary(id, resourceType))
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (c *ResourceTypePutController) fetchResourceType(ctx context.Context, id resources.ID) (*datamodel.ResourceType, error) {
	obj, err := c.StorageClient().Get(ctx, id.String())
	if err != nil {
		return nil, err
	}

	resourceType := datamodel.ResourceType{}
	err = obj.As(&resourceType)
	if err != nil {
		return nil, err
	}

	return &resourceType, nil
}

func (c *ResourceTypePutController) updateSummary(id resources.ID, resourceType *datamodel.ResourceType) func(summary *datamodel.ResourceProviderSummary) error {
	return func(summary *datamodel.ResourceProviderSummary) error {
		if summary.Properties.ResourceTypes == nil {
			summary.Properties.ResourceTypes = map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{}
		}

		resourceTypeName := id.Name()
		resourceTypeEntry, ok := summary.Properties.ResourceTypes[resourceTypeName]
		if !ok {
			resourceTypeEntry = datamodel.ResourceProviderSummaryPropertiesResourceType{}
		}

		resourceTypeEntry.DefaultAPIVersion = resourceType.Properties.DefaultAPIVersion
		summary.Properties.ResourceTypes[resourceTypeName] = resourceTypeEntry
		return nil
	}
}
