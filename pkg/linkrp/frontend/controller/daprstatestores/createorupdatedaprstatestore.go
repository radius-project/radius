// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateDaprStateStore)(nil)

// CreateOrUpdateDaprStateStore is the controller implementation to create or update DaprStateStore link resource.
type CreateOrUpdateDaprStateStore struct {
	ctrl.BaseController
}

// NewCreateOrUpdateDaprStateStore creates a new instance of CreateOrUpdateDaprStateStore.
func NewCreateOrUpdateDaprStateStore(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateDaprStateStore{ctrl.NewBaseController(opts)}, nil
}

// Run executes CreateOrUpdateDaprStateStore operation.
func (daprStateStore *CreateOrUpdateDaprStateStore) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	isSupported, err := datamodel.IsDaprInstalled(ctx, daprStateStore.KubeClient())
	if err != nil {
		return nil, err
	} else if !isSupported {
		return rest.NewBadRequestResponse(datamodel.DaprMissingError), nil
	}

	newResource, err := daprStateStore.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	old := &datamodel.DaprStateStore{}
	isNewResource := false
	etag, err := daprStateStore.GetResource(ctx, serviceCtx.ResourceID.String(), old)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			isNewResource = true
		} else {
			return nil, err
		}
	}

	if req.Method == http.MethodPatch && isNewResource {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	newResource.SystemData = ctrl.UpdateSystemData(old.SystemData, *serviceCtx.SystemData())
	if !isNewResource {
		newResource.CreatedAPIVersion = old.CreatedAPIVersion
		prop := newResource.Properties.BasicResourceProperties
		if !old.Properties.BasicResourceProperties.EqualLinkedResource(&prop) {
			return rest.NewLinkedResourceUpdateErrorResponse(serviceCtx.ResourceID, &old.Properties.BasicResourceProperties, &newResource.Properties.BasicResourceProperties), nil
		}
	}

	rendererOutput, err := daprStateStore.DeploymentProcessor().Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := daprStateStore.DeploymentProcessor().Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.Resources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues
	newResource.RecipeData = deploymentOutput.RecipeData

	if componentName, ok := deploymentOutput.ComputedValues[renderers.ComponentNameKey].(string); ok {
		newResource.Properties.ComponentName = componentName
	}

	if !isNewResource {
		diff := outputresource.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = daprStateStore.DeploymentProcessor().Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: newResource, OutputResources: diff, ComputedValues: newResource.ComputedValues, SecretValues: newResource.SecretValues, RecipeData: newResource.RecipeData})
		if err != nil {
			return nil, err
		}
	}

	savedResource, err := daprStateStore.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.DaprStateStoreDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": savedResource.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (daprStateStore *CreateOrUpdateDaprStateStore) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.DaprStateStore, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.DaprStateStoreDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = v1.ProvisioningStateSucceeded
	dm.TenantID = serviceCtx.HomeTenantID
	dm.CreatedAPIVersion = dm.UpdatedAPIVersion
	return dm, nil
}
