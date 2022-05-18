// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	planesdb "github.com/project-radius/radius/pkg/ucp/db/planes"
	"github.com/project-radius/radius/pkg/ucp/planes"
	"github.com/project-radius/radius/pkg/ucp/proxy"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

const (
	PlanesPath = "/planes"
)

//go:generate mockgen -destination=./mock_planes_ucphandler.go -package=planes -self_package github.com/project-radius/radius/pkg/ucp/frontend/ucphandler/planes github.com/project-radius/radius/pkg/ucp/frontend/ucphandler/planes PlanesUCPHandler
type PlanesUCPHandler interface {
	CreateOrUpdate(ctx context.Context, db store.StorageClient, body []byte, path string) (rest.Response, error)
	List(ctx context.Context, db store.StorageClient, path string) (rest.Response, error)
	GetByID(ctx context.Context, db store.StorageClient, path string) (rest.Response, error)
	DeleteByID(ctx context.Context, db store.StorageClient, path string) (rest.Response, error)
	ProxyRequest(ctx context.Context, db store.StorageClient, w http.ResponseWriter, r *http.Request, path string) (rest.Response, error)
}

type Options struct {
	Address string
}

// NewPlanesUCPHandler creates a new Planes UCP handler
func NewPlanesUCPHandler(options Options) PlanesUCPHandler {
	return &ucpHandler{
		options: options,
	}
}

type ucpHandler struct {
	options Options
}

func (ucp *ucpHandler) CreateOrUpdate(ctx context.Context, db store.StorageClient, body []byte, path string) (rest.Response, error) {
	var plane rest.Plane
	err := json.Unmarshal(body, &plane)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	plane.ID = path
	planeType, name, _, err := resources.ExtractPlanesPrefixFromURLPath(path)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	plane.Type = planes.PlaneTypePrefix + "/" + planeType
	plane.Name = name
	planeExists := true
	ID, err := resources.Parse(resources.UCPPrefix + plane.ID)
	//cannot parse ID something wrong with request
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// At least one provider needs to be configured

	_, err = planesdb.GetByID(ctx, db, ID)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			planeExists = false
		} else {
			return nil, err
		}
	}
	plane, err = planesdb.Save(ctx, db, plane)
	if err != nil {
		return nil, err
	}
	var restResp rest.Response
	if planeExists {
		restResp = rest.NewOKResponse(plane)
	} else {
		restResp = rest.NewCreatedResponse(plane)
	}
	return restResp, nil
}

func (ucp *ucpHandler) List(ctx context.Context, db store.StorageClient, path string) (rest.Response, error) {
	var query store.Query
	query.RootScope = resources.UCPPrefix + path
	query.ScopeRecursive = true
	query.IsScopeQuery = true
	listOfPlanes, err := planesdb.GetScope(ctx, db, query)
	if err != nil {
		return nil, err
	}
	var ok = rest.NewOKResponse(listOfPlanes)
	return ok, nil
}

func (ucp *ucpHandler) GetByID(ctx context.Context, db store.StorageClient, path string) (rest.Response, error) {
	//make id fully qualified. Ex, plane id : ucp:/planes/radius/local
	id := resources.UCPPrefix + path
	id = strings.ToLower(id)
	resourceId, err := resources.Parse(id)
	if err != nil {
		if err != nil {
			return rest.NewBadRequestResponse(err.Error()), nil
		}
	}
	plane, err := planesdb.GetByID(ctx, db, resourceId)
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
	//make id fully qualified. Ex, plane id : ucp:/planes/radius/local
	id := resources.UCPPrefix + path
	resourceId, err := resources.Parse(id)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	_, err = planesdb.GetByID(ctx, db, resourceId)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := rest.NewNoContentResponse()
			return restResponse, nil
		}
		return nil, err
	}
	err = planesdb.DeleteByID(ctx, db, resourceId)
	if err != nil {
		return nil, err
	}
	restResponse := rest.NewNoContentResponse()
	return restResponse, nil
}

func (ucp *ucpHandler) ProxyRequest(ctx context.Context, db store.StorageClient, w http.ResponseWriter, r *http.Request, path string) (rest.Response, error) {
	planeType, name, _, err := resources.ExtractPlanesPrefixFromURLPath(path)
	if err != nil {
		return rest.InternalServerError(err), err
	}

	// Lookup the plane
	planePath := PlanesPath + "/" + planeType + "/" + name
	planeID, err := resources.Parse(resources.UCPPrefix + planePath)
	if err != nil {
		if err != nil {
			return rest.InternalServerError(err), err
		}
	}
	plane, err := planesdb.GetByID(ctx, db, planeID)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			return rest.NewNotFoundResponse(planePath), err
		}
		return rest.InternalServerError(err), err
	}

	// Get the resource provider
	resourceID, err := resources.Parse(resources.UCPPrefix + path)
	if err != nil {
		return rest.InternalServerError(err), err
	}

	if resourceID.ProviderNamespace() == "" {
		err = fmt.Errorf("Invalid resourceID specified with no provider.")
		return rest.NewBadRequestResponse(err.Error()), err
	}

	// Lookup the resource providers configured to determine the URL to proxy to
	// Not using map lookups to enable case insensitive comparisons
	// We need to preserve the case while storing data in DB and therefore iterating for case
	// insensitive comparisons
	var proxyURL string
	for k, v := range plane.Properties.ResourceProviders {
		if strings.EqualFold(k, resourceID.ProviderNamespace()) {
			proxyURL = v
			break
		}
	}
	downstream, err := url.Parse(proxyURL)
	if err != nil {
		return rest.InternalServerError(err), err
	}

	options := proxy.ReverseProxyOptions{
		RoundTripper: http.DefaultTransport,
		ProxyAddress: ucp.options.Address,
	}
	ctx = context.WithValue(ctx, proxy.PlaneUrlField, proxyURL)
	ctx = context.WithValue(ctx, proxy.PlaneIdField, planePath)
	// As per https://github.com/golang/go/issues/28940#issuecomment-441749380, the way to check
	// for http vs https is check the TLS field
	httpScheme := "http"
	if r.TLS != nil {
		httpScheme = "https"
	}
	ctx = context.WithValue(ctx, proxy.HttpSchemeField, httpScheme)
	sender := proxy.NewARMProxy(options, downstream, nil)
	sender.ServeHTTP(w, r.WithContext(ctx))

	return nil, nil
}
