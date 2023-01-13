// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	fctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

var _ ctrl.Controller = (*CreateOrUpdateRedisCache)(nil)

// CreateOrUpdateRedisCache is the controller implementation to create or update RedisCache link resource.
type CreateOrUpdateRedisCache struct {
	ctrl.Operation[*datamodel.RedisCache, datamodel.RedisCache]
	dp deployment.DeploymentProcessor
}

// NewCreateOrUpdateRedisCache creates a new instance of CreateOrUpdateRedisCache.
func NewCreateOrUpdateRedisCache(opts fctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateRedisCache{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.RedisCache]{
				RequestConverter:  converter.RedisCacheDataModelFromVersioned,
				ResponseConverter: converter.RedisCacheDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateRedisCache operation.
func (rc *CreateOrUpdateRedisCache) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	newResource, err := rc.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := rc.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := rc.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := rp_frontend.PrepareRadiusResource(ctx, newResource, old, rc.Options()); r != nil || err != nil {
		return r, err
	}

	rendererOutput, err := rc.dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := rc.dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.Status.OutputResources = deploymentOutput.Resources
	newResource.ComputedValues = deploymentOutput.ComputedValues
	newResource.SecretValues = deploymentOutput.SecretValues

	if host, ok := deploymentOutput.ComputedValues[renderers.Host].(string); ok {
		newResource.Properties.Host = host
	}
	if port, ok := deploymentOutput.ComputedValues[renderers.Port]; ok {
		if port != nil {
			switch p := port.(type) {
			case float64:
				newResource.Properties.Port = int32(p)
			case int32:
				newResource.Properties.Port = p
			case string:
				converted, err := strconv.Atoi(p)
				if err != nil {
					return nil, err
				}
				newResource.Properties.Port = int32(converted)
			default:
				return nil, errors.New("unhandled type for the property port")
			}
		}
	}
	if username, ok := deploymentOutput.ComputedValues[renderers.UsernameStringValue].(string); ok {
		newResource.Properties.Username = username
	}

	if old != nil {
		diff := outputresource.GetGCOutputResources(newResource.Properties.Status.OutputResources, old.Properties.Status.OutputResources)
		err = rc.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: newResource, OutputResources: diff, ComputedValues: newResource.ComputedValues, SecretValues: newResource.SecretValues, RecipeData: newResource.RecipeData})
		if err != nil {
			return nil, err
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := rc.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return rc.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
