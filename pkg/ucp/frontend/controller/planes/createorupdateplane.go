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

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/planes"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var _ armrpc_controller.Controller = (*CreateOrUpdatePlane)(nil)

// CreateOrUpdatePlane is the controller implementation to create/update a UCP plane.
type CreateOrUpdatePlane struct {
	ctrl.BaseController
}

// NewCreateOrUpdatePlane creates a new CreateOrUpdatePlane.
func NewCreateOrUpdatePlane(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &CreateOrUpdatePlane{ctrl.NewBaseController(opts)}, nil
}

func (p *CreateOrUpdatePlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	var spanAttrKey attribute.Key
	tr := otel.Tracer("planes")
	ctx, span := tr.Start(ctx, "createOrUpdatePlane")
	defer span.End()
	req = req.WithContext(ctx)
	path := middleware.GetRelativePath(p.Options.BasePath, req.URL.Path)
	spanAttrKey = attribute.Key(middleware.UCP_REQ_URI)
	span.SetAttributes(spanAttrKey.String(path))

	body, err := controller.ReadRequestBody(req)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	apiVersion := ctrl.GetAPIVersion(req)

	spanAttrKey = attribute.Key(middleware.UCP_API)
	span.SetAttributes(spanAttrKey.String(apiVersion))

	newResource, err := converter.PlaneDataModelFromVersioned(body, apiVersion)
	if err != nil {
		span.RecordError(err)
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	_, err = resources.Parse(path)
	// cannot parse ID something wrong with request
	if err != nil {
		span.RecordError(err)
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	planeType, name, _, err := resources.ExtractPlanesPrefixFromURLPath(path)
	if err != nil {
		span.RecordError(err)
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	// Build the tracked resource
	newResource.TrackedResource = v1.TrackedResource{
		ID:   path,
		Name: name,
		Type: planes.PlaneTypePrefix + "/" + planeType,
	}

	// Check if the plane already exists
	planeExists := true
	existingResource := datamodel.Plane{}
	etag, err := p.GetResource(ctx, newResource.TrackedResource.ID, &existingResource)
	logger := logr.FromContextOrDiscard(ctx)
	if err != nil {
		span.RecordError(err)
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
		span.RecordError(err)
		return nil, err
	}

	// Return a versioned response of the plane
	versioned, err := converter.PlaneDataModelToVersioned(newResource, apiVersion)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	restResp := armrpc_rest.NewOKResponse(versioned)
	if planeExists {
		logger.Info(fmt.Sprintf("Updated plane %s successfully", newResource.TrackedResource.ID))
	} else {
		logger.Info(fmt.Sprintf("Created plane %s successfully", newResource.TrackedResource.ID))
	}
	return restResp, nil
}
