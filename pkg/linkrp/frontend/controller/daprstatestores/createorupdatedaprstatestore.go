// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	fctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*CreateOrUpdateDaprStateStore)(nil)

// CreateOrUpdateDaprStateStore is the controller implementation to create or update DaprStateStore link resource.
type CreateOrUpdateDaprStateStore struct {
	ctrl.Operation[*datamodel.DaprStateStore, datamodel.DaprStateStore]

	KubeClient runtimeclient.Client
	dp         deployment.DeploymentProcessor
}

// NewCreateOrUpdateDaprStateStore creates a new instance of CreateOrUpdateDaprStateStore.
func NewCreateOrUpdateDaprStateStore(opts fctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateDaprStateStore{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.DaprStateStore]{
				RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
				ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
		dp:         opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateDaprStateStore operation.
func (d *CreateOrUpdateDaprStateStore) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	isSupported, err := datamodel.IsDaprInstalled(ctx, d.KubeClient)
	if err != nil {
		return nil, err
	} else if !isSupported {
		return rest.NewBadRequestResponse(datamodel.DaprMissingError), nil
	}

	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	newResource, err := d.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := d.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := d.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := rp_frontend.PrepareRadiusResource(ctx, newResource, old, d.Options()); r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := d.dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := d.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.Resources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues

	if componentName, ok := deploymentOutput.ComputedValues[renderers.ComponentNameKey].(string); ok {
		newResource.Properties.ComponentName = componentName
	}

	if old != nil {
		diff := outputresource.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = d.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: newResource, OutputResources: diff, ComputedValues: newResource.ComputedValues, SecretValues: newResource.SecretValues, RecipeData: newResource.RecipeData})
		if err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := d.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return d.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
