// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/ucp/store"
	"golang.org/x/exp/slices"
	"oras.land/oras-go/v2/registry/remote"
)

var _ ctrl.Controller = (*CreateOrUpdateEnvironment)(nil)

// CreateOrUpdateEnvironments is the controller implementation to create or update environment resource.
type CreateOrUpdateEnvironment struct {
	ctrl.Operation[*datamodel.Environment, datamodel.Environment]
}

// NewCreateOrUpdateEnvironment creates a new CreateOrUpdateEnvironment.
func NewCreateOrUpdateEnvironment(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateEnvironment{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Environment]{
				RequestConverter:  converter.EnvironmentDataModelFromVersioned,
				ResponseConverter: converter.EnvironmentDataModelToVersioned,
			},
		),
	}, nil
}

// Run executes CreateOrUpdateEnvironment operation.
func (e *CreateOrUpdateEnvironment) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := e.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	// Update Recipes mapping with dev recipes.
	if newResource.Properties.UseDevRecipes {
		newResource.Properties.Recipes, err = getDevRecipes(ctx, newResource.Properties.Recipes)
		if err != nil {
			return nil, err
		}
	}

	// Create Query filter to query kubernetes namespace used by the other environment resources.
	namespace := newResource.Properties.Compute.KubernetesCompute.Namespace
	namespaceQuery := store.Query{
		RootScope:    serviceCtx.ResourceID.RootScope(),
		ResourceType: serviceCtx.ResourceID.Type(),
		Filters: []store.QueryFilter{
			{
				Field: "properties.compute.kubernetes.namespace",
				Value: namespace,
			},
		},
	}

	// Check if environment with this namespace already exists
	result, err := e.StorageClient().Query(ctx, namespaceQuery)
	if err != nil {
		return nil, err
	}

	if len(result.Items) > 0 {
		env := &datamodel.Environment{}
		if err := result.Items[0].As(env); err != nil {
			return nil, err
		}

		// If a different resource has the same namespace, return a conflict
		// Otherwise, continue and update the resource
		if env.ID != old.ID {
			return rest.NewConflictResponse(fmt.Sprintf("Environment %s with the same namespace (%s) already exists", env.ID, namespace)), nil
		}
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return e.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}

func getDevRecipes(ctx context.Context, devRecipes map[string]datamodel.EnvironmentRecipeProperties) (map[string]datamodel.EnvironmentRecipeProperties, error) {
	if devRecipes == nil {
		devRecipes = map[string]datamodel.EnvironmentRecipeProperties{}
	}

	logger := radlogger.GetLogger(ctx)
	reg, err := remote.NewRegistry(DevRecipesACRPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create client to registry %s -  %s", DevRecipesACRPath, err.Error())
	}

	// if repository has the correct path it should look like: <acrPath>/recipes/<connectorType>/<provider>
	err = reg.Repositories(ctx, "", func(repos []string) error {
		for _, repo := range repos {
			connector, provider := parseRepoPathForMetadata(repo)
			if connector != "" && provider != "" {
				if slices.Contains(supportedProviders(), provider) {
					var name string
					var connectorType string
					switch connector {
					case "mongodatabases":
						name = "mongo" + "-" + provider
						connectorType = mongodatabases.ResourceTypeName
					default:
						continue
					}
					devRecipes[name] = datamodel.EnvironmentRecipeProperties{
						ConnectorType: connectorType,
						TemplatePath:  DevRecipesACRPath + "/" + repo,
					}
				}
			}
		}

		logger.Info(fmt.Sprintf("pulled %d dev recipes", len(devRecipes)))
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list recipes available in registry at path  %s -  %s", DevRecipesACRPath, err.Error())
	}

	return devRecipes, nil
}

func parseRepoPathForMetadata(repo string) (connector string, provider string) {
	if strings.HasPrefix(repo, "recipes/") {
		recipePath := strings.Split(repo, "recipes/")[1]
		if strings.Count(recipePath, "/") == 1 {
			connector, provider := strings.Split(recipePath, "/")[0], strings.Split(recipePath, "/")[1]
			return connector, provider
		}
	}

	return connector, provider
}
