// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

var _ ctrl.ControllerInterface = (*ListEnvironments)(nil)

const maxQueryItemCount = 20

// ListEnvironments is the controller implementation to get the list of environments resources in resource group.
type ListEnvironments struct {
	ctrl.BaseController
}

// NewListEnvironments creates a new ListEnvironments.
func NewListEnvironments(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (ctrl.ControllerInterface, error) {
	return &ListEnvironments{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

func (e *ListEnvironments) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	queryItemCount, err := e.getNumberOfRecords(ctx)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	query := store.Query{
		RootScope: fmt.Sprintf("/subscriptions/%s/resourceGroup/%s",
			serviceCtx.ResourceID.SubscriptionID, serviceCtx.ResourceID.ResourceGroup),
		ResourceType: serviceCtx.ResourceID.Type(),
	}

	queryOptions := []store.QueryOptions{
		store.WithPaginationToken(serviceCtx.SkipToken),
		store.WithMaxQueryItemCount(queryItemCount),
	}

	result, err := e.DBClient.Query(ctx, query, queryOptions...)
	if err != nil {
		return nil, err
	}

	pagination, err := e.createPaginationResponse(serviceCtx.APIVersion, result)

	e.updateNextLink(ctx, req, pagination)

	return rest.NewOKResponse(pagination), err
}

// TODO: make this pagination logic generic function.
func (e *ListEnvironments) createPaginationResponse(apiversion string, result *store.ObjectQueryResult) (*armrpcv1.PaginatedList, error) {
	items := []interface{}{}
	for _, item := range result.Items {
		denv := &datamodel.Environment{}
		if err := ctrl.DecodeMap(item.Data, denv); err != nil {
			return nil, err
		}
		versioned, err := converter.EnvironmentDataModelToVersioned(denv, apiversion)
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

// getNumberOfRecords function returns the number of records requested.
// The default value is defined above.
// If there is a top query parameter, we use that instead of the default one.
func (e *ListEnvironments) getNumberOfRecords(ctx context.Context) (int, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	top := maxQueryItemCount
	var err error

	if serviceCtx.Top != "" {
		top, err = strconv.Atoi(serviceCtx.Top)
	}

	return top, err
}

// updateNextLink function updates the next link by building a URL from the request and the pagination token.
func (e *ListEnvironments) updateNextLink(ctx context.Context, req *http.Request, pagination *armrpcv1.PaginatedList) {
	if pagination.NextLink == "" {
		return
	}

	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	qps := url.Values{}
	qps.Add("api-version", serviceCtx.APIVersion)
	qps.Add("skipToken", pagination.NextLink)

	if queryItemCount, err := e.getNumberOfRecords(ctx); err == nil && serviceCtx.Top != "" {
		qps.Add("top", strconv.Itoa(queryItemCount))
	}

	pagination.NextLink = ctrl.GetURLFromReqWithQueryParameters(req, qps).String()
}
