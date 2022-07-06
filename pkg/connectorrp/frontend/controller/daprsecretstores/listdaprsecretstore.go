// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstores

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

var _ ctrl.Controller = (*ListDaprSecretStores)(nil)

// ListDaprSecretStores is the controller implementation to get the list of daprSecretStore connector resources in the resource group.
type ListDaprSecretStores struct {
	ctrl.BaseController
}

// NewListDaprSecretStores creates a new instance of ListDaprSecretStores.
func NewListDaprSecretStores(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListDaprSecretStores{ctrl.NewBaseController(opts)}, nil
}

func (daprSecretStore *ListDaprSecretStores) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	query := store.Query{
		RootScope:    serviceCtx.ResourceID.RootScope(),
		ResourceType: serviceCtx.ResourceID.Type(),
	}

	result, err := daprSecretStore.StorageClient().Query(ctx, query, store.WithPaginationToken(serviceCtx.SkipToken), store.WithMaxQueryItemCount(serviceCtx.Top))
	if err != nil {
		return nil, err
	}

	paginatedList, err := daprSecretStore.createPaginatedList(ctx, req, result)

	return rest.NewOKResponse(paginatedList), err
}

func (daprSecretStore *ListDaprSecretStores) createPaginatedList(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	items := []interface{}{}
	for _, item := range result.Items {
		dm := &datamodel.DaprSecretStore{}
		if err := item.As(dm); err != nil {
			return nil, err
		}

		versioned, err := converter.DaprSecretStoreDataModelToVersioned(dm, serviceCtx.APIVersion)
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
