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
	"strings"

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/ucp/database"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// summaryNotFoundPolicy describes the policy to follow in updateSummaryWithETag
// when the summary resource is not found.
type summaryNotFoundPolicy string

const (
	summaryNotFoundFail   summaryNotFoundPolicy = "fail"
	summaryNotFoundCreate summaryNotFoundPolicy = "create"
	summaryNotFoundIgnore summaryNotFoundPolicy = "ignore"
)

// resourceProviderSummaryIDFromRequest returns the resource provider summary ID from the resource
// id in the request. Returns the request resource ID, the summary ID, and an error.
//
// This function handles cases where the resource ID is a resource provider ID or one of the child-types
// like a location.
func resourceProviderSummaryIDFromRequest(request *ctrl.Request) (resources.ID, resources.ID, error) {
	id, err := resources.ParseResource(request.ResourceID)
	if err != nil {
		return resources.ID{}, resources.ID{}, err
	}

	// We need to find the first type segment of the ID. It should match the resource provider type.
	if !strings.EqualFold(id.TypeSegments()[0].Type, datamodel.ResourceProviderResourceType) {
		return resources.ID{}, resources.ID{}, fmt.Errorf("expected resource provider id or an id for a child-type of resource provider, got %q", id)
	}

	summaryID, err := datamodel.ResourceProviderSummaryIDFromParts(id.RootScope(), id.TypeSegments()[0].Name)
	if err != nil {
		return resources.ID{}, resources.ID{}, err
	}

	return id, summaryID, nil
}

// updateResourceProviderSummaryWithETag updates the summary with the provided function and saves it to the database client.
func updateResourceProviderSummaryWithETag(ctx context.Context, client database.Client, summaryID resources.ID, policy summaryNotFoundPolicy, update func(summary *datamodel.ResourceProviderSummary) error) error {
	// There are a few cases here:
	// 1. The summary does not exist and we are allowed to create it (in the resource provider).
	// 2. The summary does not exist and we are not allowed to create it (in the child-types of resource provider).
	// 3. Any other error case.
	summary := &datamodel.ResourceProviderSummary{}

	obj, err := client.Get(ctx, summaryID.String())
	if errors.Is(err, &database.ErrNotFound{}) && policy == summaryNotFoundCreate {
		// This is fine. We will create a new summary.
		summary.ID = summaryID.String()
		summary.Name = summaryID.Name()
		summary.Type = summaryID.Type()

		obj = &database.Object{
			Metadata: database.Metadata{
				ID: summary.ID,
			},
		}
	} else if errors.Is(err, &database.ErrNotFound{}) && policy == summaryNotFoundIgnore {
		return nil
	} else if errors.Is(err, &database.ErrNotFound{}) {
		return err
	} else if err != nil {
		return err
	} else {
		err = obj.As(summary)
		if err != nil {
			return err
		}
	}

	// At this point obj and summary should both be populated - run the provided
	// function to update it.
	err = update(summary)
	if err != nil {
		return err
	}

	// Now we can save. Use the ETag if the resource already existed.
	options := []database.SaveOptions{}
	if obj.ETag != "" {
		options = append(options, database.WithETag(obj.ETag))
	}

	obj.Data = summary
	err = client.Save(ctx, obj, options...)
	if err != nil {
		return err
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Updated resource provider summary", "id", summaryID.String(), "data", summary)

	return nil
}
