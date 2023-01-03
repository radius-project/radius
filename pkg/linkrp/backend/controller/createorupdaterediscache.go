// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/linkrp/backend/binders"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/rp/validation"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateRedisCache)(nil)

// CreateOrUpdateRedisCache is the async operation controller to update Applications.Link/redisCache resource.
type CreateOrUpdateRedisCache struct {
	ctrl.BaseController
	engine  recipes.Engine
	auth    *armauth.ArmConfig
	binders map[string]binders.Binder[*datamodel.RedisCache]
}

// NewCreateOrUpdateRedisCache creates the CreateOrUpdateRedisCache controller instance.
func NewCreateOrUpdateRedisCache(opts ctrl.Options, engine recipes.Engine, auth *armauth.ArmConfig) (ctrl.Controller, error) {
	return &CreateOrUpdateRedisCache{
		BaseController: ctrl.NewBaseAsyncController(opts),
		engine:         engine,
		auth:           auth,
		binders: map[string]binders.Binder[*datamodel.RedisCache]{
			"Microsoft.Cache/redis": &binders.RedisAzureBinder{},
		},
	}, nil
}

// Run execute background processing for the redis ressource.
//
// Run is responsible for (in order):
//
// - Looking up the current state of the resource (data model)
// - Executing a recipe (if needed)
// - Binding the provided recipe, resource, or values and validating the result
// - Garbage collecting stale resources
// - Saving the result and making the processing complete
func (c *CreateOrUpdateRedisCache) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	redis, etag, err := c.GetResource(ctx, request.ResourceID)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
	}

	// Processing a recipe involves running some provisioning code (usually a bicep template)
	// and then passing the result into the binder.
	var recipeResult *recipes.Result
	if redis.Properties.Mode == datamodel.LinkModeRecipe {
		recipeResult, err = c.ExecuteRecipe(ctx, redis)
		if err != nil {
			return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
		}
	}

	oldResources := redis.Properties.Status.OutputResources
	err = c.PopulateLink(ctx, redis, recipeResult)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
	}

	err = c.GarbageCollect(ctx, redis, oldResources)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
	}

	_, err = c.SaveResource(ctx, request.ResourceID, redis, *etag)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
	}

	return ctrl.Result{}, nil
}

func (c *CreateOrUpdateRedisCache) ExecuteRecipe(ctx context.Context, redis *datamodel.RedisCache) (*recipes.Result, error) {
	recipe := recipes.Recipe{
		Name:          redis.Properties.Recipe.Name,
		ApplicationID: redis.Properties.Application,
		EnvironmentID: redis.Properties.Environment,
		ResourceID:    redis.ID,
		Parameters:    redis.Properties.Recipe.Parameters,
	}

	return c.engine.Execute(ctx, recipe)
}

func (c *CreateOrUpdateRedisCache) PopulateLink(ctx context.Context, redis *datamodel.RedisCache, recipeResult *recipes.Result) error {
	// The algorithm for populating a link works as follows:
	//
	// 1. Process a recipe result and extract all resources from it (mode == recipe) OR
	//	  Extract the 'resource' field (mode == resource).
	// 2. Select and run a resource binder. This will populate fields on the resource and secret references.
	// 3. Copy values and secrets from the recipe result (mode == recipe) OR
	//	  Copy values and secrets from the datamodel (mode == values).
	//
	//    This is done after step 1 & 2 because it ensures that values provided explicitly by the user
	//    will take precedence over the defaults produced by the binder.
	//
	// 4. Apply defaults for anything not yet set.

	secretValues := map[string]rp.SecretValueReference{}
	outputResources := []outputresource.OutputResource{}

	if redis.Properties.Mode == datamodel.LinkModeRecipe {
		for _, resource := range recipeResult.Resources {
			parsed, err := resources.ParseResource(resource)
			if err != nil {
				return conv.NewClientErrInvalidRequest(fmt.Sprintf("resource id %q returned by recipe is invalid", resource))
			}

			outputResources = append(outputResources, outputresource.FromUCPID(parsed))
		}
	} else if redis.Properties.Mode == datamodel.LinkModeResource {
		id, err := resources.ParseResource(redis.Properties.Resource)
		if err != nil {
			return conv.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
		}

		outputResources = append(outputResources, outputresource.FromUCPID(id))
	}

	for _, resource := range outputResources {
		binder, ok := c.binders[resource.ResourceType.Type]
		if !ok && redis.Properties.Mode == datamodel.LinkModeResource {
			supported := []string{}
			for key := range c.binders {
				supported = append(supported, key)
			}
			sort.Strings(supported)

			return conv.NewClientErrInvalidRequest(fmt.Sprintf("%q is not a support resource type. Supported types: %s", resource.ResourceType.Type, strings.Join(supported, ", ")))
		} else if ok {
			err := binder.Bind(ctx, resource.Identity.GetID(), c.Fetch, redis, secretValues)
			if err != nil {
				return err
			}

			// Bind to the first resource we support
			break
		}
	}

	if redis.Properties.Mode == datamodel.LinkModeRecipe {
		validator := &validation.Validator{}
		validator.AssignStringFromMap(&redis.Properties.Host, renderers.Host, recipeResult.Values, "recipe")
		validator.AssignInt32FromMap(&redis.Properties.Port, renderers.Port, recipeResult.Values, "recipe")
		validator.AssignStringFromMap(&redis.Properties.Username, renderers.UsernameStringValue, recipeResult.Values, "recipe", validation.Optional())

		// All of these secrets are optional.
		connectionString, password, url := "", "", ""
		if validator.AssignStringFromMap(&connectionString, renderers.ConnectionStringValue, recipeResult.Secrets, "recipe", validation.Optional()) {
			secretValues[renderers.ConnectionStringValue] = rp.SecretValueReference{Value: connectionString}
		}
		if validator.AssignStringFromMap(&password, renderers.PasswordStringHolder, recipeResult.Secrets, "recipe", validation.Optional()) {
			secretValues[renderers.ConnectionStringValue] = rp.SecretValueReference{Value: password}
		}
		if validator.AssignStringFromMap(&url, renderers.URLValue, recipeResult.Secrets, "recipe", validation.Optional()) {
			secretValues[renderers.ConnectionStringValue] = rp.SecretValueReference{Value: url}
		}

		if validator.HasErrors() {
			return conv.NewClientErrInvalidRequest(validator.FormatError())
		}
	} else if redis.Properties.Mode == datamodel.LinkModeValues {
		// Due to a quirk of how secret values are handled, we should only set these when they are  not the zero-value.
		if redis.Properties.Secrets.ConnectionString != "" {
			secretValues[renderers.ConnectionStringValue] = rp.SecretValueReference{Value: redis.Properties.Secrets.ConnectionString}
		}

		if redis.Properties.Secrets.Password != "" {
			secretValues[renderers.PasswordStringHolder] = rp.SecretValueReference{Value: redis.Properties.Secrets.Password}
		}

		if redis.Properties.Secrets.URL != "" {
			secretValues[renderers.URLValue] = rp.SecretValueReference{Value: redis.Properties.Secrets.URL}
		}
	}

	redis.Properties.BasicResourceProperties.Status.OutputResources = outputResources
	redis.SecretValues = secretValues

	// TODO: .ComputedValues needs to be set here for compat with existing designs.
	redis.ComputedValues = map[string]interface{}{
		renderers.Host:                redis.Properties.Host,
		renderers.Port:                redis.Properties.Port,
		renderers.UsernameStringValue: redis.Properties.Username,
	}

	// Now we can fill in the computed secrets based on what we know so far.
	ssl := redis.Properties.Port == 6380
	protocol := "redis"
	if ssl {
		protocol = "rediss"
	}

	// Password is OPTIONAL. Many redis configurations have no authentication.
	var password *string
	pwholder, ok := secretValues[renderers.PasswordStringHolder]
	if ok {
		password = &pwholder.Value
	}

	if _, ok := secretValues[renderers.ConnectionStringValue]; !ok {
		connectionString := fmt.Sprintf("%s:%v,abortConnect=False", redis.Properties.Host, redis.Properties.Port)
		if ssl {
			connectionString = connectionString + ",ssl=True"
		}
		if redis.Properties.Username != "" && password != nil {
			connectionString = connectionString + ",user=" + redis.Properties.Username + ",password=" + *password
		}

		secretValues["connectionString"] = rp.SecretValueReference{Value: connectionString}
	}

	if _, ok := secretValues[renderers.URLValue]; !ok {
		if redis.Properties.Username == "" && redis.Properties.Secrets.Password == "" {
			secretValues["url"] = rp.SecretValueReference{Value: fmt.Sprintf("%s://%s:%v", protocol, redis.Properties.Host, redis.Properties.Port)}
		} else {
			secretValues["url"] = rp.SecretValueReference{Value: fmt.Sprintf("%s://%s:%s@%s:%v", protocol, redis.Properties.Username, *password, redis.Properties.Host, redis.Properties.Port)}
		}
	}

	return nil
}

func (c *CreateOrUpdateRedisCache) GarbageCollect(ctx context.Context, redis *datamodel.RedisCache, old []outputresource.OutputResource) error {
	id, err := resources.ParseResource(redis.ID)
	if err != nil {
		return err
	}

	diff := outputresource.GetGCOutputResources(redis.Properties.Status.OutputResources, old)
	err = c.LinkDeploymentProcessor().Delete(ctx, deployment.ResourceData{ID: id, Resource: redis, OutputResources: diff, ComputedValues: redis.ComputedValues, SecretValues: redis.SecretValues, RecipeData: redis.RecipeData})
	if err != nil {
		return err
	}

	return nil
}

func (c *CreateOrUpdateRedisCache) Fetch(ctx context.Context, obj interface{}, id string, apiVersion string) error {
	parsed, err := resources.ParseResource(id)
	if err != nil {
		return err
	}

	rc := clients.NewGenericResourceClient(parsed.FindScope(resources.SubscriptionsSegment), c.auth.Auth)
	resource, err := rc.GetByID(ctx, id, apiVersion)
	if err != nil {
		return fmt.Errorf("failed to access resource %q", id)
	}

	// We turn the resource into a weakly-typed representation. This is needed because JSON Pointer
	// will have trouble with the autorest embdedded types.
	b, err := json.Marshal(&resource)
	if err != nil {
		return fmt.Errorf("failed to marshal %T", resource)
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		return errors.New("failed to umarshal resource data")
	}

	return nil
}

func (c *CreateOrUpdateRedisCache) GetResource(ctx context.Context, id string) (*datamodel.RedisCache, *string, error) {
	obj, err := c.StorageClient().Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	redis := datamodel.RedisCache{}
	err = obj.As(&redis)
	if err != nil {
		return nil, nil, err
	}

	return &redis, &obj.ETag, nil
}

func (c *CreateOrUpdateRedisCache) SaveResource(ctx context.Context, id string, in *datamodel.RedisCache, etag string) (*store.Object, error) {
	nr := &store.Object{
		Metadata: store.Metadata{
			ID: id,
		},
		Data: in,
	}
	err := c.StorageClient().Save(ctx, nr, store.WithETag(etag))
	if err != nil {
		return nil, err
	}
	return nr, nil
}
