// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"encoding/json"
	"errors"
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
	DeleteByID(ctx context.Context, db store.StorageClient, path string) (rest.Response, error)
}

// NewResourceGroupsUCPHandler creates a new UCP handler
func NewResourceGroupsUCPHandler() ResourceGroupsUCPHandler {
	return &ucpHandler{}
}

type ucpHandler struct {
}

func (ucp *ucpHandler) Create(ctx context.Context, db store.StorageClient, body []byte, path string) (rest.Response, error) {
	var rg rest.ResourceGroup
	err := json.Unmarshal(body, &rg)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	rg.ID = path
	rgExists := true
	ID, err := resources.Parse(resources.UCPPrefix + rg.ID)
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
	query.RootScope = resources.UCPPrefix + path
	query.ScopeRecursive = false
	query.IsScopeQuery = true
	listOfPlanes, err := resourcegroupsdb.GetScope(ctx, db, query)
	if err != nil {
		return nil, err
	}
	var ok = rest.NewOKResponse(listOfPlanes)
	return ok, nil
}

func (ucp *ucpHandler) GetByID(ctx context.Context, db store.StorageClient, path string) (rest.Response, error) {
	//make id fully qualified. Ex, plane id : ucp:/planes/radius/local/resourceGroups/rg
	id := resources.UCPPrefix + path
	id = strings.ToLower(id)
	resourceId, err := resources.Parse(id)
	if err != nil {
		if err != nil {
			return rest.NewBadRequestResponse(err.Error()), nil
		}
	}
	plane, err := resourcegroupsdb.GetByID(ctx, db, resourceId)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := rest.NewNotFoundResponse(path)
			return restResponse, nil
		}
		return nil, err
	}
	restResponse := rest.NewOKResponse(plane)
	return restResponse, nil
}

func (ucp *ucpHandler) DeleteByID(ctx context.Context, db store.StorageClient, path string) (rest.Response, error) {
	//make id fully qualified. Ex, plane id : ucp:/planes/radius/local/resourceGroups/rg
	id := resources.UCPPrefix + path
	resourceId, err := resources.Parse(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	_, err = resourcegroupsdb.GetByID(ctx, db, resourceId)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := rest.NewNoContentResponse()
			return restResponse, nil
		}
		return nil, err
	}
	err = resourcegroupsdb.DeleteByID(ctx, db, resourceId)
	if err != nil {
		return nil, err
	}
	restResponse := rest.NewNoContentResponse()
	return restResponse, nil
}
