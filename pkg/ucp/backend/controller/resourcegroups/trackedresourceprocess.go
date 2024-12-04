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
	"net/http"
	"net/url"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/pkg/ucp/trackedresource"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*TrackedResourceProcessController)(nil)

type updater interface {
	Update(ctx context.Context, downstreamURL string, originalID resources.ID, version string) error
}

// TrackedResourceProcessController is the async operation controller to perform background processing on tracked resources.
type TrackedResourceProcessController struct {
	ctrl.BaseController

	// Updater is the utility struct that can perform updates on tracked resources. This can be modified for testing.
	updater updater

	// defaultDownstream is the address of the dynamic resource provider to proxy requests to.
	defaultDownstream *url.URL
}

// NewTrackedResourceProcessController creates a new TrackedResourceProcessController controller which is used to process resources asynchronously.
func NewTrackedResourceProcessController(opts ctrl.Options, transport http.RoundTripper, defaultDownstream *url.URL) (ctrl.Controller, error) {
	return &TrackedResourceProcessController{
		BaseController:    ctrl.NewBaseAsyncController(opts),
		updater:           trackedresource.NewUpdater(opts.StorageClient, &http.Client{Transport: transport}),
		defaultDownstream: defaultDownstream,
	}, nil
}

// Run retrieves a resource from storage, parses the resource ID, and updates our tracked resource entry in the background.
func (c *TrackedResourceProcessController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	resource, err := store.GetResource[datamodel.GenericResource](ctx, c.StorageClient(), request.ResourceID)
	if errors.Is(err, &store.ErrNotFound{}) {
		return ctrl.NewFailedResult(v1.ErrorDetails{Code: v1.CodeNotFound, Message: fmt.Sprintf("resource %q not found", request.ResourceID), Target: request.ResourceID}), nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	originalID, err := resources.Parse(resource.Properties.ID)
	if err != nil {
		return ctrl.Result{}, err
	}

	downstreamURL, err := resourcegroups.ValidateDownstream(ctx, c.StorageClient(), originalID, v1.LocationGlobal, resource.Properties.APIVersion)
	if errors.Is(err, &resourcegroups.NotFoundError{}) {
		return ctrl.NewFailedResult(v1.ErrorDetails{Code: v1.CodeNotFound, Message: err.Error(), Target: request.ResourceID}), nil
	} else if errors.Is(err, &resourcegroups.InvalidError{}) {
		return ctrl.NewFailedResult(v1.ErrorDetails{Code: v1.CodeInvalid, Message: err.Error(), Target: request.ResourceID}), nil
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to validate downstream: %w", err)
	}

	if downstreamURL == nil {
		downstreamURL = c.defaultDownstream
	}

	if downstreamURL == nil {
		message := "No downstream address was configured for the resource provider, and no default downstream address was provided"
		return ctrl.NewFailedResult(v1.ErrorDetails{Code: v1.CodeInvalid, Message: message, Target: resource.Properties.ID}), nil
	}

	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Processing tracked resource", "resourceID", originalID)

	err = c.updater.Update(ctx, downstreamURL.String(), originalID, resource.Properties.APIVersion)
	if errors.Is(err, &trackedresource.InProgressErr{}) {
		// The resource is still being processed, so we can sleep for a while.
		result := ctrl.Result{}
		result.SetFailed(v1.ErrorDetails{Code: v1.CodeConflict, Message: err.Error(), Target: request.ResourceID}, true)

		return result, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Completed processing tracked resource", "resourceID", originalID)
	return ctrl.Result{}, nil
}
