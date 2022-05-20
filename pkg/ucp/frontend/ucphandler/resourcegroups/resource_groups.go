// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	planesdb "github.com/project-radius/radius/pkg/ucp/db/planes"
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

const (
	APIVersionQueryParam         = "api-version"
	DefaultAPIVersionForRadiusRP = "2022-03-15-privatepreview" // For now, hardcoding the API version for the RP.
	ResourceGroupsIdentifier     = "/resourcegroups"
)

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
	ID, err := resources.Parse(resources.UCPPrefix + rg.ID)
	//cannot parse ID something wrong with request
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// TODO: Validate resource group name

	existing, err := resourcegroupsdb.GetByID(ctx, db, ID)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			rgExists = false
		} else {
			return nil, err
		}
	}

	if rgExists {
		// Copy over the read only properties
		rg.ID = existing.ID
		rg.Name = existing.Name
		rg.ProvisioningState = existing.ProvisioningState
	}
	// Block PUT operations when delete is in progress
	if rg.ProvisioningState == rest.ProvisioningStateDeleting {
		return rest.NewConflictResponse("Cannot create/update resource group while delete is in progress"), nil
	}

	// Update provisioning state
	rg.ProvisioningState = rest.ProvisioningStateSucceeded
	_, err = resourcegroupsdb.Save(ctx, db, rg)
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
	listOfResources, err := resourcegroupsdb.GetScopeRecursive(ctx, db, query)
	if err != nil {
		return rest.ResourceList{}, err
	}
	return listOfResources, nil
}

func (ucp *ucpHandler) GetByID(ctx context.Context, db store.StorageClient, path string) (rest.Response, error) {
	//make id fully qualified. Ex, plane id : ucp:/planes/radius/local/resourceGroups/rg
	id := resources.UCPPrefix + path
	id = strings.ToLower(id)
	resourceID, err := resources.Parse(id)
	if err != nil {
		if err != nil {
			return rest.NewBadRequestResponse(err.Error()), nil
		}
	}
	plane, err := resourcegroupsdb.GetByID(ctx, db, resourceID)
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

func (ucp *ucpHandler) DeleteByID(ctx context.Context, db store.StorageClient, path string, req *http.Request) (rest.Response, error) {
	//make id fully qualified. Ex, plane id : ucp:/planes/radius/local/resourceGroups/rg
	id := resources.UCPPrefix + path
	resourceID, err := resources.Parse(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	rg, err := resourcegroupsdb.GetByID(ctx, db, resourceID)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := rest.NewNoContentResponse()
			return restResponse, nil
		}
		return nil, err
	}

	// Delete resource group issues individual delete requests for all resources in the resource group
	// and then deletes the resource group entry from the DB

	// Persist the provisioning state to block PUT operations
	rg.ProvisioningState = rest.ProvisioningStateDeleting
	resourcegroupsdb.Save(ctx, db, rg)

	plane, err := getPlane(ctx, db, id)
	if err != nil {
		return nil, err
	}

	// Get all resources under the path with resource group prefix
	listOfResources, err := ucp.listResources(ctx, db, path)
	if err != nil {
		return nil, err
	}

	for _, r := range listOfResources.Value {
		// Lookup the provider URL
		segments := strings.Split(r.Type, resources.SegmentSeparator)
		provider := strings.ToLower(segments[0])
		providerURL := plane.LookupResourceProvider(provider)
		if providerURL == "" {
			return nil, fmt.Errorf("Provider %s is not configured with plane for %s", provider, id)
		}

		// Construct and make a delete request to the provider
		deleteReq, err := constructDeleteRequest(providerURL, r, req)
		if err != nil {
			return nil, err
		}

		client := ucp.options.Client
		if client == nil {
			client = http.DefaultClient
		}

		// TODO: Handle async deletes by the RP
		_, err = client.Do(deleteReq)
		if err != nil {
			return nil, err
		}
	}

	err = resourcegroupsdb.DeleteByID(ctx, db, resourceID)
	if err != nil {
		return nil, err
	}
	restResponse := rest.NewNoContentResponse()
	return restResponse, nil
}

func constructDeleteRequest(providerURL string, r rest.Resource, req *http.Request) (*http.Request, error) {
	path := providerURL + r.ID
	deleteReq, err := http.NewRequest(http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}

	values := req.URL.Query()
	// TODO: Figure out the api version
	values.Add(APIVersionQueryParam, DefaultAPIVersionForRadiusRP)
	deleteReq.URL.RawQuery = values.Encode()
	deleteReq.Host = providerURL

	return deleteReq, nil
}

func getPlane(ctx context.Context, db store.StorageClient, id string) (rest.Plane, error) {
	// Read the providers for the plane
	segments := strings.Split(strings.ToLower(id), ResourceGroupsIdentifier)
	planeID, err := resources.Parse(segments[0])
	if err != nil {
		return rest.Plane{}, err
	}

	plane, err := planesdb.GetByID(ctx, db, planeID)
	if err != nil {
		return rest.Plane{}, err
	}
	return plane, nil
}
