// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstores

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	kube "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*CreateOrUpdateDaprSecretStore)(nil)

// CreateOrUpdateDaprSecretStore is the controller implementation to create or update DaprSecretStore link resource.
type CreateOrUpdateDaprSecretStore struct {
	ctrl.Operation[*datamodel.DaprSecretStore, datamodel.DaprSecretStore]
	KubeClient kube.Client
	dp         deployment.DeploymentProcessor
}

// NewCreateOrUpdateDaprSecretStore creates a new instance of CreateOrUpdateDaprSecretStore.
func NewCreateOrUpdateDaprSecretStore(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateDaprSecretStore{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.DaprSecretStore]{
				RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
				ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
		dp:         opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateDaprSecretStore operation.
func (daprSecretStore *CreateOrUpdateDaprSecretStore) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	isSupported, err := datamodel.IsDaprInstalled(ctx, daprSecretStore.KubeClient)
	if err != nil {
		return nil, err
	} else if !isSupported {
		return rest.NewBadRequestResponse(datamodel.DaprMissingError), nil
	}

	newResource, err := daprSecretStore.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := daprSecretStore.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	r, err := daprSecretStore.PrepareResource(ctx, req, newResource, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	r, err = rp_frontend.PrepareRadiusResource(ctx, newResource, old, daprSecretStore.Options())
	if r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := daprSecretStore.dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}

	deploymentOutput, err := daprSecretStore.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.Status.OutputResources = deploymentOutput.DeployedOutputResources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues
	if componentName, ok := deploymentOutput.ComputedValues[linkrp.ComponentNameKey].(string); ok {
		newResource.Properties.ComponentName = componentName
	}

	if old != nil {
		diff := rpv1.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = daprSecretStore.dp.Delete(ctx, serviceCtx.ResourceID, diff)
		if err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := daprSecretStore.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return daprSecretStore.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
