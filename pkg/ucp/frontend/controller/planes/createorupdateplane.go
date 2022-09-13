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
	http "net/http"

	"github.com/project-radius/radius/pkg/middleware"
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
	body, err := controller.ReadRequestBody(req)
	if err != nil {
		return nil, err
	}

	path := middleware.GetRelativePath(p.Options.BasePath, req.URL.Path)
	var plane rest.Plane
	err = json.Unmarshal(body, &plane)
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
	id, err := resources.Parse(plane.ID)
	//cannot parse ID something wrong with request
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	ctx = ucplog.WrapLogContext(ctx, ucplog.LogFieldPlaneKind, plane.Properties.Kind)
	logger := ucplog.GetLogger(ctx)
	// At least one provider needs to be configured
	if plane.Properties.Kind == rest.PlaneKindUCPNative {
		if plane.Properties.ResourceProviders == nil || len(plane.Properties.ResourceProviders) == 0 {
			err = fmt.Errorf("At least one resource provider must be configured for UCP native plane: %s", plane.Name)
			return rest.NewBadRequestResponse(err.Error()), nil
		}
	} else if plane.Properties.Kind != rest.PlaneKindAWS {
		if plane.Properties.URL == "" {
			err = fmt.Errorf("URL must be specified for plane: %s", plane.Name)
			return rest.NewBadRequestResponse(err.Error()), nil
		}
	}

	planeExists := true
	existingPlane := rest.Plane{}
	etag, err := p.GetResource(ctx, id.String(), &existingPlane)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			planeExists = false
			logger.Info(fmt.Sprintf("No existing plane %s found in db", id))
		} else {
			return nil, err
		}
	}

	_, err = p.SaveResource(ctx, id.String(), plane, etag)
	if err != nil {
		return nil, err
	}
	restResp := rest.NewOKResponse(plane)
	if planeExists {
		logger.Info(fmt.Sprintf("Updated plane %s successfully", plane.Name))
	} else {
		logger.Info(fmt.Sprintf("Created plane %s successfully", plane.Name))
	}
	return restResp, nil
}
