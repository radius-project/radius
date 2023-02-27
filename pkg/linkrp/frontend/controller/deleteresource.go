// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// DeleteResource is the controller implementation to delete a link resource.
type DeleteResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any] struct {
	ctrl.Operation[P, T]
	dp deployment.DeploymentProcessor
}

// NewDeleteResource creates a new instance DeleteResource.
func NewDeleteResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any](opts Options, op ctrl.Operation[P, T]) (ctrl.Controller, error) {
	return &DeleteResource[P, T]{
		Operation: op,
		dp:        opts.DeployProcessor,
	}, nil
}

func (link *DeleteResource[P, T]) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := link.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	r, err := link.PrepareResource(ctx, req, nil, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	oldP := P(old)
	err = link.dp.Delete(ctx, serviceCtx.ResourceID, oldP.OutputResources())
	if err != nil {
		return nil, err
	}

	err = link.StorageClient().Delete(ctx, serviceCtx.ResourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
