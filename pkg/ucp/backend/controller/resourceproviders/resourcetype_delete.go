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
	"errors"
	"fmt"

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*ResourceTypeDeleteController)(nil)

// ResourceTypeDeleteController is the async operation controller to perform DELETE operations on resource types.
type ResourceTypeDeleteController struct {
	ctrl.BaseController

	// Connection is the connection to UCP.
	Connection sdk.Connection
}

// Run implements the controller interface.
func (c *ResourceTypeDeleteController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	err := c.deleteChildResources(ctx, request)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete child resources: %w", err)
	}

	id, summaryID, err := resourceProviderSummaryIDFromRequest(request)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = updateResourceProviderSummaryWithETag(ctx, c.StorageClient(), summaryID, summaryNotFoundIgnore, c.updateSummary(id))
	if err != nil {
		return ctrl.Result{}, err
	}

	err = c.StorageClient().Delete(ctx, request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (c *ResourceTypeDeleteController) deleteChildResources(ctx context.Context, request *ctrl.Request) error {
	// Cascading delete of child resources (apiVersions).
	apiVersions, err := c.apiVersions(ctx, request.ResourceID)
	if err != nil {
		return err
	}

	// We don't do retries here because we're already in a retry loop in the parent controller.
	var deleteErrors []error
	for _, apiVersion := range apiVersions {
		err := c.deleteApiVersion(ctx, apiVersion)
		if err != nil {
			// Attempt deletion of all child resources before returning an error.
			//
			// This will avoid head-of-line blocking in the retry loop in the parent controller.
			deleteErrors = append(deleteErrors, err)
		}
	}

	if len(deleteErrors) > 0 {
		return errors.Join(deleteErrors...)
	}

	return nil
}

func (c *ResourceTypeDeleteController) apiVersions(ctx context.Context, rawID string) ([]*v20231001preview.APIVersionResource, error) {
	id, err := resources.ParseResource(rawID)
	if err != nil {
		return nil, err
	}

	client, err := v20231001preview.NewAPIVersionsClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.Connection))
	if err != nil {
		return nil, err
	}

	results := []*v20231001preview.APIVersionResource{}
	pager := client.NewListPager(id.FindScope(resources_radius.PlaneTypeRadius), id.TypeSegments()[0].Name, id.TypeSegments()[1].Name, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		results = append(results, page.Value...)
	}

	return results, nil
}

func (c *ResourceTypeDeleteController) deleteApiVersion(ctx context.Context, apiVersion *v20231001preview.APIVersionResource) error {
	id, err := resources.ParseResource(*apiVersion.ID)
	if err != nil {
		return err
	}

	client, err := v20231001preview.NewAPIVersionsClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.Connection))
	if err != nil {
		return err
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Beginning cascading delete of API version", "id", id.String())
	poller, err := client.BeginDelete(
		ctx,
		id.FindScope(resources_radius.PlaneTypeRadius),
		id.TypeSegments()[0].Name,
		id.TypeSegments()[1].Name,
		id.Name(),
		nil)
	if err != nil {
		return fmt.Errorf("failed to delete API version %s: %w", id.String(), err)
	}

	_, err = poller.Poll(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete API version %s: %w", id.String(), err)
	}

	logger.Info("Completed cascading delete of API version", "id", id.String())
	return nil
}

func (c *ResourceTypeDeleteController) updateSummary(id resources.ID) func(summary *datamodel.ResourceProviderSummary) error {
	return func(summary *datamodel.ResourceProviderSummary) error {
		resourceTypeName := id.Name()
		delete(summary.Properties.ResourceTypes, resourceTypeName)

		return nil
	}
}
