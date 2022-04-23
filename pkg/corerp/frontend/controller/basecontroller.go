// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

// ControllerInterface is an interface of each operation controller.
type ControllerInterface interface {
	// Run executes the operation.
	Run(ctx context.Context, req *http.Request) (rest.Response, error)
}

// BaseController is the base operation controller.
type BaseController struct {
	// TODO: db.RadrpDB and deployment.DeploymentProcessor will be replaced with new implementation.
	DBClient  store.StorageClient
	JobEngine deployment.DeploymentProcessor
}

// GetResource is the helper to get the resource via storage client.
func (c *BaseController) GetResource(ctx context.Context, id string, out interface{}) (etag string, err error) {
	etag = ""
	var res *store.Object
	if res, err = c.DBClient.Get(ctx, id); err == nil {
		if err = DecodeMap(res.Data, out); err == nil {
			etag = res.ETag
			return
		}
	}
	return
}

// SaveResource is the helper to save the resource via storage client.
func (c *BaseController) SaveResource(ctx context.Context, id string, in interface{}, etag string) error {
	newObject := &store.Object{
		Metadata: store.Metadata{
			ID: id,
		},
		Data: in,
	}
	if _, err := c.DBClient.Save(ctx, newObject, store.WithETag(etag)); err != nil {
		return err
	}
	return nil
}

// CreatePaginationResponse is the helper to create the paginated response.
func (c *BaseController) CreatePaginationResponse(ctx context.Context, result *store.ObjectQueryResult) (*armrpcv1.PaginatedList, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	items := []interface{}{}
	for _, environ := range result.Items {
		denv := &datamodel.Environment{}
		if err := DecodeMap(environ.Data, denv); err != nil {
			return nil, err
		}
		versioned, err := converter.EnvironmentDataModelToVersioned(denv, serviceCtx.APIVersion)
		if err != nil {
			return nil, err
		}

		items = append(items, versioned)
	}

	// TODO: implement pagination using paginationtoken
	return &armrpcv1.PaginatedList{
		Value: items,
		// TODO: set NextLink: if result.PaginationToken is not empty
	}, nil
}

// UpdateSystemData creates or updates new systemdata from old and new resources.
func UpdateSystemData(old armrpcv1.SystemData, new armrpcv1.SystemData) armrpcv1.SystemData {
	newSystemData := old

	if old.CreatedAt == "" && new.CreatedAt != "" {
		newSystemData.CreatedAt = new.CreatedAt
		newSystemData.CreatedBy = new.CreatedBy
		newSystemData.CreatedByType = new.CreatedByType
	}

	if new.LastModifiedAt != "" {
		newSystemData.LastModifiedAt = new.LastModifiedAt
		newSystemData.LastModifiedBy = new.LastModifiedBy
		newSystemData.LastModifiedByType = new.LastModifiedByType

		// backfill
		if newSystemData.CreatedAt == "" {
			newSystemData.CreatedAt = new.LastModifiedAt
			newSystemData.CreatedBy = new.LastModifiedBy
			newSystemData.CreatedByType = new.LastModifiedByType
		}
	}

	return newSystemData
}
