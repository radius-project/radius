// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"
	"fmt"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/engine"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

// CreateOrUpdateResource is the async operation controller to create or update Applications.Link resources.
type CreateOrUpdateResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any] struct {
	ctrl.BaseController
	processor processors.ResourceProcessor[P, T]
	engine       engine.Engine
	client    processors.ResourceClient
}

// NewCreateOrUpdateResource creates the CreateOrUpdateResource controller instance.
//
// The processor function will be called to process updates to the resource.
func NewCreateOrUpdateResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any](processor processors.ResourceProcessor[P, T], eng engine.Engine, client processors.ResourceClient, opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateResource[P, T]{ctrl.NewBaseAsyncController(opts), processor, eng, client}, nil
}

func (c *CreateOrUpdateResource[P, T]) Run(ctx context.Context, req *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.StorageClient().Get(ctx, req.ResourceID)
	if errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.Result{}, err
	} else if err != nil {
		return ctrl.Result{}, err
	}

	data := P(new(T))
	if err = obj.As(data); err != nil {
		return ctrl.Result{}, err
	}

	// Clone existing output resources so we can diff them later.
	previousOutputResources := c.copyOutputResources(data)

	// Now we're ready to process recipes (if needed).
	recipeOutput, err := c.executeRecipeIfNeeded(ctx, data)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Now we're ready to process the resource. This will handle the updates to any user-visible state.
	err = c.processor.Process(ctx, data, recipeOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Now we need to clean up any obsolete output resources.
	diff := rpv1.GetGCOutputResources(data.OutputResources(), previousOutputResources)
	err = c.garbageCollectResources(ctx, req.ResourceID, diff)
	if err != nil {
		return ctrl.Result{}, err
	}

	update := &store.Object{
		Metadata: store.Metadata{
			ID: req.ResourceID,
		},
		Data: data,
	}
	err = c.StorageClient().Save(ctx, update, store.WithETag(obj.ETag))
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

func (c *CreateOrUpdateResource[P, T]) copyOutputResources(data P) []rpv1.OutputResource {
	previousOutputResources := make([]rpv1.OutputResource, len(data.OutputResources()))
	copy(previousOutputResources, data.OutputResources())
	return previousOutputResources
}

func (c *CreateOrUpdateResource[P, T]) executeRecipeIfNeeded(ctx context.Context, data P) (*recipes.RecipeOutput, error) {
	// 'any' is required here to convert to an interface type, only then can we use a type assertion.
	recipeDataModel, supportsRecipes := any(data).(datamodel.RecipeDataModel)
	if !supportsRecipes {
		return nil, nil
	}

	input := recipeDataModel.Recipe()
	request := recipes.Metadata{
		Name:          input.Name,
		Parameters:    input.Parameters,
		EnvironmentID: data.ResourceMetadata().Environment,
		ApplicationID: data.ResourceMetadata().Application,
		ResourceID:    data.GetBaseResource().ID,
	}

	return c.engine.Execute(ctx, request)
}

func (c *CreateOrUpdateResource[P, T]) garbageCollectResources(ctx context.Context, id string, diff []rpv1.OutputResource) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	for _, resource := range diff {
		id := resource.Identity.GetID()
		logger.Info(fmt.Sprintf("Deleting output resource: %q", id), ucplog.LogFieldTargetResourceID, id)
		err := c.client.Delete(ctx, id)
		if err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("Deleted output resource: %q", id), ucplog.LogFieldTargetResourceID, id)
	}

	return nil
}
