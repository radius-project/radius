// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"

	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ base_ctrl.ControllerInterface = (*ListRedisCaches)(nil)

// ListRedisCaches is the controller implementation to get the list of rediscache connector resources in the resource group.
type ListRedisCaches struct {
	base_ctrl.BaseController
}

// NewListRedisCaches creates a new instance of ListRedisCaches.
func NewListRedisCaches(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (base_ctrl.ControllerInterface, error) {
	return &ListRedisCaches{
		BaseController: base_ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

func (redis *ListRedisCaches) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	query := store.Query{
		RootScope:    serviceCtx.ResourceID.RootScope(),
		ResourceType: serviceCtx.ResourceID.Type(),
	}

	result, err := redis.DBClient.Query(ctx, query, store.WithPaginationToken(serviceCtx.SkipToken), store.WithMaxQueryItemCount(serviceCtx.Top))
	if err != nil {
		return nil, err
	}

	paginatedList, err := redis.createPaginatedList(ctx, req, result)

	return rest.NewOKResponse(paginatedList), err
}

func (redis *ListRedisCaches) createPaginatedList(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*armrpcv1.PaginatedList, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	items := []interface{}{}
	for _, item := range result.Items {
		dm := &datamodel.RedisCache{}
		if err := base_ctrl.DecodeMap(item.Data, dm); err != nil {
			return nil, err
		}

		versioned, err := converter.RedisCacheDataModelToVersioned(dm, serviceCtx.APIVersion)
		if err != nil {
			return nil, err
		}

		items = append(items, versioned)
	}

	return &armrpcv1.PaginatedList{
		Value:    items,
		NextLink: base_ctrl.GetNextLinkURL(ctx, req, result.PaginationToken),
	}, nil
}
