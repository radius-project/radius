// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"

	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListRedisCaches)(nil)

// ListRedisCaches is the controller implementation to get the list of rediscache link resources in the resource group.
type ListRedisCaches struct {
	ctrl.BaseController
}

// NewListRedisCaches creates a new instance of ListRedisCaches.
func NewListRedisCaches(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListRedisCaches{ctrl.NewBaseController(opts)}, nil
}

func (redis *ListRedisCaches) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	query := store.Query{
		RootScope:    serviceCtx.ResourceID.RootScope(),
		ResourceType: serviceCtx.ResourceID.Type(),
	}

	result, err := redis.StorageClient().Query(ctx, query, store.WithPaginationToken(serviceCtx.SkipToken), store.WithMaxQueryItemCount(serviceCtx.Top))
	if err != nil {
		return nil, err
	}

	paginatedList, err := redis.createPaginatedList(ctx, req, result)

	return rest.NewOKResponse(paginatedList), err
}

func (redis *ListRedisCaches) createPaginatedList(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	items := []interface{}{}
	for _, item := range result.Items {
		dm := &datamodel.RedisCache{}
		if err := item.As(dm); err != nil {
			return nil, err
		}

		versioned, err := converter.RedisCacheDataModelToVersioned(dm, serviceCtx.APIVersion, false)
		if err != nil {
			return nil, err
		}

		items = append(items, versioned)
	}

	return &v1.PaginatedList{
		Value:    items,
		NextLink: ctrl.GetNextLinkURL(ctx, req, result.PaginationToken),
	}, nil
}
