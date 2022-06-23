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
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateDaprStateStore)(nil)

// CreateOrUpdateDaprStateStore is the controller implementation to create or update DaprStateStore connector resource.
type CreateOrUpdateDaprStateStore struct {
	ctrl.BaseController
}

// NewCreateOrUpdateDaprStateStore creates a new instance of CreateOrUpdateDaprStateStore.
func NewCreateOrUpdateDaprStateStore(ds store.StorageClient, sm manager.StatusManager) (ctrl.Controller, error) {
	return &CreateOrUpdateDaprStateStore{ctrl.NewBaseController(ds, sm)}, nil
}

// Run executes CreateOrUpdateDaprStateStore operation.
func (daprStateStore *CreateOrUpdateDaprStateStore) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := daprStateStore.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	// TODO Integrate with renderer/deployment processor to validate associated resource existence (if fromResource is defined)
	// and store resource properties and secrets reference

	// Read existing resource info from the data store
	existingResource := &datamodel.DaprStateStore{}
	etag, err := daprStateStore.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	// Add system metadata to requested resource
	newResource.SystemData = ctrl.UpdateSystemData(existingResource.SystemData, *serviceCtx.SystemData())
	if existingResource.CreatedAPIVersion != "" {
		newResource.CreatedAPIVersion = existingResource.CreatedAPIVersion
	}
	newResource.TenantID = serviceCtx.HomeTenantID

	// Add/update resource in the data store
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
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
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
	daprStateStoreProperties := dm.Properties.GetDaprStateStoreProperties()
	daprStateStoreProperties.ProvisioningState = v1.ProvisioningStateSucceeded
	switch v := dm.Properties.(type) {
	case *datamodel.DaprStateStoreAzureTableStorageResourceProperties:
		dm.Properties = &datamodel.DaprStateStoreAzureTableStorageResourceProperties{
			DaprStateStoreProperties: daprStateStoreProperties,
			Resource:                 v.Resource,
		}
	case *datamodel.DaprStateStoreSQLServerResourceProperties:
		dm.Properties = &datamodel.DaprStateStoreSQLServerResourceProperties{
			DaprStateStoreProperties: daprStateStoreProperties,
			Resource:                 v.Resource,
		}
	case *datamodel.DaprStateStoreGenericResourceProperties:
		dm.Properties = &datamodel.DaprStateStoreGenericResourceProperties{
			DaprStateStoreProperties: daprStateStoreProperties,
			Type:                     v.Type,
			Version:                  v.Version,
			Metadata:                 v.Metadata,
		}
	default:
		dm.Properties = daprStateStoreProperties
	}
	return dm, nil
}
