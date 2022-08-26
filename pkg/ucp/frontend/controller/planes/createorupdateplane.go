// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"errors"
	"fmt"
	http "net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/planes"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*CreateOrUpdatePlane)(nil)

// CreateOrUpdatePlane is the controller implementation to create/update a UCP plane.
type CreateOrUpdatePlane struct {
	ctrl.BaseController
}

// NewCreateOrUpdatePlane creates a new CreateOrUpdatePlane.
func NewCreateOrUpdatePlane(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdatePlane{ctrl.NewBaseController(opts)}, nil
}

func (p *CreateOrUpdatePlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	path := middleware.GetRelativePath(p.Options.BasePath, req.URL.Path)

	body, err := controller.ReadRequestBody(req)
	if err != nil {
		return nil, err
	}

	logger := ucplog.GetLogger(ctx)
	apiVersion := ctrl.GetAPIVersion(logger, req)

	newResource, err := converter.PlaneDataModelFromVersioned(body, apiVersion)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// Validate the request
	err = Validate(path, newResource)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	planeType, name, _, err := resources.ExtractPlanesPrefixFromURLPath(path)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// Build the tracked resource
	newResource.TrackedResource = v1.TrackedResource{
		ID:   path,
		Name: name,
		Type: planes.PlaneTypePrefix + "/" + planeType,
	}

	ctx = ucplog.WrapLogContext(ctx, ucplog.LogFieldPlaneKind, newResource.Properties.Kind)
	logger = ucplog.GetLogger(ctx)

	// Check if the plane already exists
	planeExists := true
	existingResource := datamodel.Plane{}
	etag, err := p.GetResource(ctx, newResource.TrackedResource.ID, &existingResource)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			planeExists = false
			logger.Info(fmt.Sprintf("No existing plane %s found in db", newResource.TrackedResource.ID))
		} else {
			return nil, err
		}
	}

	// Save the data model plane to the database
	_, err = p.SaveResource(ctx, newResource.TrackedResource.ID, *newResource, etag)
	if err != nil {
		return nil, err
	}

	// Return a versioned response of the plane
	versioned, err := converter.PlaneDataModelToVersioned(newResource, apiVersion)
	if err != nil {
		return nil, err
	}

	restResp := rest.NewOKResponse(versioned)
	if planeExists {
		logger.Info(fmt.Sprintf("Updated plane %s successfully", newResource.TrackedResource.ID))
	} else {
		logger.Info(fmt.Sprintf("Created plane %s successfully", newResource.TrackedResource.ID))
	}
	return restResp, nil
}

func Validate(path string, dmp *datamodel.Plane) error {
	var err error
	if dmp.Properties.Kind == rest.PlaneKindUCPNative {
		// At least one provider needs to be configured
		if dmp.Properties.ResourceProviders == nil || len(dmp.Properties.ResourceProviders) == 0 {
			err = fmt.Errorf("At least one resource provider must be configured for UCP native plane: %s", dmp.TrackedResource.Name)
			return err
		}
	} else {
		// The URL must be specified for non-native UCP Plane
		if dmp.Properties.URL == nil || len(*dmp.Properties.URL) == 0 {
			err = fmt.Errorf("URL must be specified for plane: %s", dmp.TrackedResource.Name)
			return err
		}
	}

	_, err = resources.Parse(path)
	// cannot parse ID something wrong with request
	if err != nil {
		return err
	}
	return nil
}
