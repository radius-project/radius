// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"

	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListMongoDatabases)(nil)

// ListMongoDatabases is the controller implementation to get the list of mongodatabase connector resources in the resource group.
type ListMongoDatabases struct {
	ctrl.BaseController
}

// NewListMongoDatabases creates a new instance of ListMongoDatabases.
func NewListMongoDatabases(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListMongoDatabases{ctrl.NewBaseController(opts)}, nil
}

func (mongo *ListMongoDatabases) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	query := store.Query{
		RootScope:    serviceCtx.ResourceID.RootScope(),
		ResourceType: serviceCtx.ResourceID.Type(),
	}

	result, err := mongo.StorageClient().Query(ctx, query, store.WithPaginationToken(serviceCtx.SkipToken), store.WithMaxQueryItemCount(serviceCtx.Top))
	if err != nil {
		return nil, err
	}

	paginatedList, err := mongo.createPaginatedList(ctx, req, result)

	return rest.NewOKResponse(paginatedList), err
}

func (mongo *ListMongoDatabases) createPaginatedList(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	items := []interface{}{}
	for _, item := range result.Items {
		dm := &datamodel.MongoDatabaseResponse{}
		if err := item.As(dm); err != nil {
			return nil, err
		}

		versioned, err := converter.MongoDatabaseDataModelToVersioned(dm, serviceCtx.APIVersion, false)
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
