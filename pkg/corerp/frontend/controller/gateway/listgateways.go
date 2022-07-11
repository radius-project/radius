// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListGateways)(nil)

// ListGateways is the controller implementation to get the list of gateway resources in resource group.
type ListGateways struct {
	ctrl.BaseController
}

// NewListGateways creates a new ListGateways.
func NewListGateways(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListGateways{ctrl.NewBaseController(opts)}, nil
}

func (g *ListGateways) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	query := store.Query{
		RootScope:    serviceCtx.ResourceID.RootScope(),
		ResourceType: serviceCtx.ResourceID.Type(),
	}

	result, err := g.StorageClient().Query(ctx, query, store.WithPaginationToken(serviceCtx.SkipToken), store.WithMaxQueryItemCount(serviceCtx.Top))
	if err != nil {
		return nil, err
	}

	pagination, err := g.createPaginationResponse(ctx, req, result)

	return rest.NewOKResponse(pagination), err
}

// TODO: make this pagination logic generic function.
func (g *ListGateways) createPaginationResponse(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	items := []interface{}{}
	for _, item := range result.Items {
		dGtwy := &datamodel.Gateway{}
		if err := item.As(dGtwy); err != nil {
			return nil, err
		}
		versioned, err := converter.GatewayDataModelToVersioned(dGtwy, serviceCtx.APIVersion)
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
