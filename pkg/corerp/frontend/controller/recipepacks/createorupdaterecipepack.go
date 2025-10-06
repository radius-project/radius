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

package recipepacks

import (
	"context"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*CreateOrUpdateRecipePack)(nil)

// CreateOrUpdateRecipePack is the controller implementation to create or update recipe pack resource.
type CreateOrUpdateRecipePack struct {
	ctrl.Operation[*datamodel.RecipePack, datamodel.RecipePack]
}

// NewCreateOrUpdateRecipePack creates a new controller for creating or updating a recipe pack resource.
func NewCreateOrUpdateRecipePack(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateRecipePack{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.RecipePack]{
				RequestConverter:  converter.RecipePackDataModelFromVersioned,
				ResponseConverter: converter.RecipePackDataModelToVersioned,
			},
		),
	}, nil
}

// Run creates or updates a recipe pack resource.
func (r *CreateOrUpdateRecipePack) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := r.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := r.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if resp, err := r.PrepareResource(ctx, req, newResource, old, etag); resp != nil || err != nil {
		return resp, err
	}

	logger.Info("Creating or updating recipe pack", "resourceID", serviceCtx.ResourceID.String())

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := r.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return r.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
