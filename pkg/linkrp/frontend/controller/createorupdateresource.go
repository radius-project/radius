// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOrUpdateResource is the controller implementation to create or update any link resource.
type CreateOrUpdateResource[P interface {
	*T
	datamodel.Link
}, T any] struct {
	ctrl.Operation[P, T]
	KubeClient kube.Client
	dp         deployment.DeploymentProcessor
	isDapr     bool
}

// NewCreateOrUpdateResource creates the CreateOrUpdateResource controller instance.
func NewCreateOrUpdateResource[P interface {
	*T
	datamodel.Link
}, T any](opts Options, op ctrl.Operation[P, T], isDapr bool) (ctrl.Controller, error) {
	return &CreateOrUpdateResource[P, T]{
		Operation:  op,
		KubeClient: opts.KubeClient,
		dp:         opts.DeployProcessor,
		isDapr:     isDapr,
	}, nil
}

// Run executes CreateOrUpdateResource operation.
func (link *CreateOrUpdateResource[P, T]) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	if link.isDapr {
		isSupported, err := datamodel.IsDaprInstalled(ctx, link.KubeClient)
		if err != nil {
			return nil, err
		} else if !isSupported {
			return rest.NewBadRequestResponse(datamodel.DaprMissingError), nil
		}
	}

	newResource, err := link.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := link.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	r, err := link.PrepareResource(ctx, req, newResource, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	r, err = rp_frontend.PrepareRadiusResource[P](ctx, newResource, old, link.Options())
	if r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := link.dp.Render(ctx, serviceCtx.ResourceID, P(newResource))
	if err != nil {
		return nil, err
	}

	deploymentOutput, err := link.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	err = P(newResource).ApplyDeploymentOutput(deploymentOutput)
	if err != nil {
		return nil, err
	}

	if old != nil {
		diff := rpv1.GetGCOutputResources(P(newResource).OutputResources(), P(old).OutputResources())
		err = link.dp.Delete(ctx, serviceCtx.ResourceID, diff)
		if err != nil {
			return nil, err
		}
	}

	P(newResource).SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := link.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return link.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
