// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprinvokehttproutes

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers/daprinvokehttproutes"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*CreateOrUpdateDaprInvokeHttpRoute)(nil)

// CreateOrUpdateDaprInvokeHttpRoute is the controller implementation to create or update DaprInvokeHttpRoute link resource.
type CreateOrUpdateDaprInvokeHttpRoute struct {
	ctrl.Operation[*datamodel.DaprInvokeHttpRoute, datamodel.DaprInvokeHttpRoute]
	KubeClient kube.Client
	dp         deployment.DeploymentProcessor
}

// NewCreateOrUpdateDaprInvokeHttpRoute creates a new instance of CreateOrUpdateDaprInvokeHttpRoute.
func NewCreateOrUpdateDaprInvokeHttpRoute(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateDaprInvokeHttpRoute{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.DaprInvokeHttpRoute]{
				RequestConverter:  converter.DaprInvokeHttpRouteDataModelFromVersioned,
				ResponseConverter: converter.DaprInvokeHttpRouteDataModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
		dp:         opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateDaprInvokeHttpRoute operation.
func (daprHttpRoute *CreateOrUpdateDaprInvokeHttpRoute) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	isSupported, err := datamodel.IsDaprInstalled(ctx, daprHttpRoute.KubeClient)
	if err != nil {
		return nil, err
	} else if !isSupported {
		return rest.NewBadRequestResponse(datamodel.DaprMissingError), nil
	}

	newResource, err := daprHttpRoute.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := daprHttpRoute.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	r, err := daprHttpRoute.PrepareResource(ctx, req, newResource, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	r, err = rp_frontend.PrepareRadiusResource(ctx, newResource, old, daprHttpRoute.Options())
	if r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := daprHttpRoute.dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}

	deploymentOutput, err := daprHttpRoute.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.Status.OutputResources = deploymentOutput.Resources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues
	if appId, ok := deploymentOutput.ComputedValues[daprinvokehttproutes.AppIDKey].(string); ok {
		newResource.Properties.AppId = appId
	}

	if old != nil {
		diff := outputresource.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = daprHttpRoute.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: newResource, OutputResources: diff, ComputedValues: newResource.ComputedValues, SecretValues: newResource.SecretValues, RecipeData: newResource.RecipeData})
		if err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := daprHttpRoute.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return daprHttpRoute.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
