// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprinvokehttproutes

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

var _ ctrl.Controller = (*ListDaprInvokeHttpRoutes)(nil)

// ListDaprInvokeHttpRoutes is the controller implementation to get the list of daprInvokeHttpRoute link resources in the resource group.
type ListDaprInvokeHttpRoutes struct {
	ctrl.BaseController
}

// NewListDaprInvokeHttpRoutes creates a new instance of ListDaprInvokeHttpRoutes.
func NewListDaprInvokeHttpRoutes(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListDaprInvokeHttpRoutes{ctrl.NewBaseController(opts)}, nil
}

func (daprHttpRoute *ListDaprInvokeHttpRoutes) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	query := store.Query{
		RootScope:    serviceCtx.ResourceID.RootScope(),
		ResourceType: serviceCtx.ResourceID.Type(),
	}

	result, err := daprHttpRoute.StorageClient().Query(ctx, query, store.WithPaginationToken(serviceCtx.SkipToken), store.WithMaxQueryItemCount(serviceCtx.Top))
	if err != nil {
		return nil, err
	}

	paginatedList, err := daprHttpRoute.createPaginatedList(ctx, req, result)

	return rest.NewOKResponse(paginatedList), err
}

func (daprHttpRoute *ListDaprInvokeHttpRoutes) createPaginatedList(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	items := []interface{}{}
	for _, item := range result.Items {
		dm := &datamodel.DaprInvokeHttpRoute{}
		if err := item.As(dm); err != nil {
			return nil, err
		}

		versioned, err := converter.DaprInvokeHttpRouteDataModelToVersioned(dm, serviceCtx.APIVersion)
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
