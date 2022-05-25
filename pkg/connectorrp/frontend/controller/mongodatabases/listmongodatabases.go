// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

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

var _ base_ctrl.ControllerInterface = (*ListMongoDatabases)(nil)

// ListMongoDatabases is the controller implementation to get the list of mongodatabase connector resources in the resource group.
type ListMongoDatabases struct {
	base_ctrl.BaseController
}

// NewListMongoDatabases creates a new instance of ListMongoDatabases.
func NewListMongoDatabases(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (base_ctrl.ControllerInterface, error) {
	return &ListMongoDatabases{
		BaseController: base_ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

func (mongo *ListMongoDatabases) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	query := store.Query{
		RootScope:    serviceCtx.ResourceID.RootScope(),
		ResourceType: serviceCtx.ResourceID.Type(),
	}

	result, err := mongo.DBClient.Query(ctx, query, store.WithPaginationToken(serviceCtx.SkipToken), store.WithMaxQueryItemCount(serviceCtx.Top))
	if err != nil {
		return nil, err
	}

	paginatedList, err := mongo.createPaginatedList(ctx, req, result)

	return rest.NewOKResponse(paginatedList), err
}

func (mongo *ListMongoDatabases) createPaginatedList(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*armrpcv1.PaginatedList, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	items := []interface{}{}
	for _, item := range result.Items {
		dm := &datamodel.MongoDatabase{}
		if err := base_ctrl.DecodeMap(item.Data, dm); err != nil {
			return nil, err
		}

		versioned, err := converter.MongoDatabaseDataModelToVersioned(dm, serviceCtx.APIVersion)
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
