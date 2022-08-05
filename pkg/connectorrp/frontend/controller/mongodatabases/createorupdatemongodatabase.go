// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateMongoDatabase)(nil)

// CreateOrUpdateMongoDatabase is the controller implementation to create or update MongoDatabase connector resource.
type CreateOrUpdateMongoDatabase struct {
	ctrl.BaseController
}

// NewCreateOrUpdateMongoDatabase creates a new instance of CreateOrUpdateMongoDatabase.
func NewCreateOrUpdateMongoDatabase(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateMongoDatabase{ctrl.NewBaseController(opts)}, nil
}

// Run executes CreateOrUpdateMongoDatabase operation.
func (mongo *CreateOrUpdateMongoDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := mongo.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	old := &datamodel.MongoDatabase{}
	isNewResource := false
	etag, err := mongo.GetResource(ctx, serviceCtx.ResourceID.String(), old)
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
		if !old.Properties.BasicResourceProperties.EqualLinkedResource(prop) {
			return rest.NewLinkedResourceUpdateErrorResponse(serviceCtx.ResourceID, &old.Properties.BasicResourceProperties, &newResource.Properties.BasicResourceProperties), nil
		}
	}

	rendererOutput, err := mongo.DeploymentProcessor().Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := mongo.DeploymentProcessor().Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.Resources
	newResource.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
	newResource.InternalMetadata.SecretValues = deploymentOutput.SecretValues

	if database, ok := deploymentOutput.ComputedValues[renderers.DatabaseNameValue].(string); ok {
		newResource.Properties.Database = database
	}

	if !isNewResource {
		diff := outputresource.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = mongo.DeploymentProcessor().Delete(ctx, serviceCtx.ResourceID, diff)
		if err != nil {
			return nil, err
		}
	}

	savedResource, err := mongo.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	mongoResponse := &datamodel.MongoDatabaseResponse{}
	err = savedResource.As(mongoResponse)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.MongoDatabaseDataModelToVersioned(mongoResponse, serviceCtx.APIVersion, false)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": savedResource.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (mongo *CreateOrUpdateMongoDatabase) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.MongoDatabase, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.MongoDatabaseDataModelFromVersioned(content, apiVersion)
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
