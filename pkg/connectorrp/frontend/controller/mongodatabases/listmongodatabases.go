// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/project-radius/radius/pkg/api/armrpcv1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"

	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

var _ base_ctrl.ControllerInterface = (*ListMongoDatabases)(nil)

const defaultQueryItemCount = 20

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

	queryItemCount, err := mongo.getNumberOfRecords(ctx)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	rID := serviceCtx.ResourceID

	query := store.Query{
		RootScope:    fmt.Sprintf("/subscriptions/%s/resourceGroup/%s", rID.SubscriptionID, rID.ResourceGroup),
		ResourceType: rID.Type(),
	}

	result, err := mongo.DBClient.Query(ctx, query, store.WithPaginationToken(serviceCtx.SkipToken), store.WithMaxQueryItemCount(queryItemCount))
	if err != nil {
		return nil, err
	}

	paginatedList, err := mongo.createPaginatedList(serviceCtx.APIVersion, result)

	mongo.updateNextLink(ctx, req, paginatedList)

	return rest.NewOKResponse(paginatedList), err
}

func (mongo *ListMongoDatabases) createPaginatedList(apiversion string, result *store.ObjectQueryResult) (*armrpcv1.PaginatedList, error) {
	items := []interface{}{}
	for _, item := range result.Items {
		dm := &datamodel.MongoDatabase{}
		if err := base_ctrl.DecodeMap(item.Data, dm); err != nil {
			return nil, err
		}

		versioned, err := converter.MongoDatabaseDataModelToVersioned(dm, apiversion)
		if err != nil {
			return nil, err
		}

		items = append(items, versioned)
	}

	return &armrpcv1.PaginatedList{
		Value:    items,
		NextLink: result.PaginationToken,
	}, nil
}

// TODO this will be abstracted out after https://github.com/project-radius/radius/pull/2319
// getNumberOfRecords returns the number of records requested.
func (mongo *ListMongoDatabases) getNumberOfRecords(ctx context.Context) (int, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	top := defaultQueryItemCount
	var err error

	if serviceCtx.Top != "" {
		top, err = strconv.Atoi(serviceCtx.Top)
	}

	return top, err
}

// updateNextLink updates the next link by building a URL from the request and the pagination token.
func (mongo *ListMongoDatabases) updateNextLink(ctx context.Context, req *http.Request, pagination *armrpcv1.PaginatedList) {
	if pagination.NextLink == "" {
		return
	}

	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	qps := url.Values{}
	qps.Add("api-version", serviceCtx.APIVersion)
	qps.Add("skipToken", pagination.NextLink)

	if queryItemCount, err := mongo.getNumberOfRecords(ctx); err == nil && serviceCtx.Top != "" {
		qps.Add("top", strconv.Itoa(queryItemCount))
	}

	pagination.NextLink = base_ctrl.GetURLFromReqWithQueryParameters(req, qps).String()
}
