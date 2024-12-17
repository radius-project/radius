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

var _ ctrl.Controller = (*LocationDeleteController)(nil)

// LocationDeleteController is the async operation controller to perform DELETE operations on locations.
type LocationDeleteController struct {
	ctrl.BaseController
}

// Run implements the controller interface.
func (c *LocationDeleteController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	id, summaryID, err := resourceProviderSummaryIDFromRequest(request)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = updateResourceProviderSummaryWithETag(ctx, c.DatabaseClient(), summaryID, summaryNotFoundIgnore, c.updateSummary(id))
	if err != nil {
		return ctrl.Result{}, err
	}

	err = c.DatabaseClient().Delete(ctx, request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (c *LocationDeleteController) updateSummary(id resources.ID) func(summary *datamodel.ResourceProviderSummary) error {
	return func(summary *datamodel.ResourceProviderSummary) error {
		locationName := id.Name()
		delete(summary.Properties.Locations, locationName)

		return nil
	}
}
