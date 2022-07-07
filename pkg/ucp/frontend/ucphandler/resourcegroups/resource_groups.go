// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	resourcegroupsdb "github.com/project-radius/radius/pkg/ucp/db/resourcegroups"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

//go:generate mockgen -destination=./mock_resourcegroups_ucphandler.go -package=resourcegroups -self_package github.com/project-radius/radius/pkg/ucp/frontend/ucphandler/resourcegroups github.com/project-radius/radius/pkg/ucp/frontend/ucphandler/resourcegroups ResourceGroupsUCPHandler
type ResourceGroupsUCPHandler interface {
	Create(ctx context.Context, db store.StorageClient, body []byte, path string) (rest.Response, error)
	List(ctx context.Context, db store.StorageClient, path string) (rest.Response, error)
	GetByID(ctx context.Context, db store.StorageClient, path string) (rest.Response, error)
	DeleteByID(ctx context.Context, db store.StorageClient, path string, request *http.Request) (rest.Response, error)
}

type Options struct {
	Address  string
	BasePath string
	Client   *http.Client
}

// NewResourceGroupsUCPHandler creates a new UCP handler
func NewResourceGroupsUCPHandler(options Options) ResourceGroupsUCPHandler {
	return &ucpHandler{
		options: options,
	}
}

type ucpHandler struct {
	options Options
}

func (ucp *ucpHandler) Create(ctx context.Context, db store.StorageClient, body []byte, path string) (rest.Response, error) {
	var rg rest.ResourceGroup
	err := json.Unmarshal(body, &rg)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	rg.ID = path
	rgExists := true
	ID, err := resources.Parse(rg.ID)
	//cannot parse ID something wrong with request
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// TODO: Validate resource group name

	_, err = resourcegroupsdb.GetByID(ctx, db, ID)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			rgExists = false
		} else {
			return nil, err
		}
	}

	rg.Name = ID.Name()
	rg, err = resourcegroupsdb.Save(ctx, db, rg)
	if err != nil {
		return nil, err
	}

	var restResp rest.Response
	if rgExists {
		restResp = rest.NewOKResponse(rg)
	} else {
		restResp = rest.NewCreatedResponse(rg)
	}
	return restResp, nil
}

func (ucp *ucpHandler) List(ctx context.Context, db store.StorageClient, path string) (rest.Response, error) {
	var query store.Query
	planeType, planeName, _, err := resources.ExtractPlanesPrefixFromURLPath(path)
	if err != nil {
		return nil, err
	}
	query.RootScope = resources.SegmentSeparator + resources.PlanesSegment + resources.SegmentSeparator + planeType + resources.SegmentSeparator + planeName
	query.IsScopeQuery = true
	query.ResourceType = "resourcegroups"
	listOfResourceGroups, err := resourcegroupsdb.GetScope(ctx, db, query)
	if err != nil {
		return nil, err
	}
	var ok = rest.NewOKResponse(listOfResourceGroups)
	return ok, nil
}

func (ucp *ucpHandler) listResources(ctx context.Context, db store.StorageClient, path string) (rest.ResourceList, error) {
	var query store.Query
	query.RootScope = path
	query.ScopeRecursive = true
	query.IsScopeQuery = false
	listOfResources, err := resourcegroupsdb.GetScopeAllResources(ctx, db, query)
	if err != nil {
		return rest.ResourceList{}, err
	}
	return listOfResources, nil
}

func (ucp *ucpHandler) GetByID(ctx context.Context, db store.StorageClient, path string) (rest.Response, error) {
	id := strings.ToLower(path)
	resourceID, err := resources.Parse(id)
	if err != nil {
		if err != nil {
			return rest.NewBadRequestResponse(err.Error()), nil
		}
	}
	rg, err := resourcegroupsdb.GetByID(ctx, db, resourceID)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := rest.NewNotFoundResponse(path)
			return restResponse, nil
		}
		return nil, err
	}
	restResponse := rest.NewOKResponse(rg)
	return restResponse, nil
}

func (ucp *ucpHandler) DeleteByID(ctx context.Context, db store.StorageClient, path string, request *http.Request) (rest.Response, error) {
	resourceID, err := resources.Parse(path)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	_, err = resourcegroupsdb.GetByID(ctx, db, resourceID)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := rest.NewNoContentResponse()
			return restResponse, nil
		}
		return nil, err
	}

	// Get all resources under the path with resource group prefix
	listOfResources, err := ucp.listResources(ctx, db, path)
	if err != nil {
		return nil, err
	}

	if len(listOfResources.Value) != 0 {
		return rest.NewConflictResponse("Resource group is not empty and cannot be deleted"), nil
	}

	err = resourcegroupsdb.DeleteByID(ctx, db, resourceID)
	if err != nil {
		return nil, err
	}
	restResponse := rest.NewNoContentResponse()
	return restResponse, nil
}
