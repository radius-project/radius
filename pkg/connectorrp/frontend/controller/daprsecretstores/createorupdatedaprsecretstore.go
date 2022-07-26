// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstores

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateDaprSecretStore)(nil)

// CreateOrUpdateDaprSecretStore is the controller implementation to create or update DaprSecretStore connector resource.
type CreateOrUpdateDaprSecretStore struct {
	ctrl.BaseController
}

// NewCreateOrUpdateDaprSecretStore creates a new instance of CreateOrUpdateDaprSecretStore.
func NewCreateOrUpdateDaprSecretStore(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateDaprSecretStore{ctrl.NewBaseController(opts)}, nil
}

// Run executes CreateOrUpdateDaprSecretStore operation.
func (daprSecretStore *CreateOrUpdateDaprSecretStore) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := daprSecretStore.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	rendererOutput, err := daprSecretStore.DeploymentProcessor().Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := daprSecretStore.DeploymentProcessor().Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.Resources
	newResource.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
	newResource.InternalMetadata.SecretValues = deploymentOutput.SecretValues
	if secretStoreName, ok := deploymentOutput.ComputedValues[renderers.SecretStoreName].(string); ok {
		newResource.Properties.SecretStoreName = secretStoreName
	}

	// Read existing resource info from the data store
	old := &datamodel.DaprSecretStore{}
	isNewResource := false
	etag, err := daprSecretStore.GetResource(ctx, serviceCtx.ResourceID.String(), old)
	if errors.Is(&store.ErrNotFound{}, err) {
		isNewResource = true
	}
	if err != nil && !isNewResource {
		return nil, err
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
		if !old.Properties.BasicResourceProperties.EqualParentResource(prop) {
			return rest.NewBadRequestResponse(fmt.Sprintf(ctrl.MismatchedParentResourceMessageFormat, prop.Application, prop.Environment)), nil
		}
	}

	// Add/update resource in the data store
	savedResource, err := daprSecretStore.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.DaprSecretStoreDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": savedResource.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (daprSecretStore *CreateOrUpdateDaprSecretStore) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.DaprSecretStore, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.DaprSecretStoreDataModelFromVersioned(content, apiVersion)
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
