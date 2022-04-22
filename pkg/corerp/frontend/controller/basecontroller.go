// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
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

func UpdateSystemData(old armrpcv1.SystemData, new armrpcv1.SystemData) armrpcv1.SystemData {
	newSystemData := old

	if old.CreatedAt == "" {
		newSystemData.CreatedAt = new.CreatedAt
		newSystemData.CreatedBy = new.CreatedBy
		newSystemData.CreatedByType = new.CreatedByType
	}

	newSystemData.LastModifiedAt = new.LastModifiedAt
	newSystemData.LastModifiedBy = new.LastModifiedBy
	newSystemData.LastModifiedByType = new.LastModifiedByType

	// fallback
	if newSystemData.CreatedAt == "" {
		newSystemData.CreatedAt = new.LastModifiedAt
		newSystemData.CreatedBy = new.LastModifiedBy
		newSystemData.CreatedByType = new.LastModifiedByType
	}

	return newSystemData
}
