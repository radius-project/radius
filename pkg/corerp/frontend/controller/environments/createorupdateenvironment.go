// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/frontend/controller/util"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"golang.org/x/exp/maps"
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

	if err := newResource.Properties.Compute.Identity.Validate(); err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// Update Recipes mapping with dev recipes.
	if newResource.Properties.UseDevRecipes {
		devRecipes, err := getDevRecipes(ctx)
		if err != nil {
			return nil, err
		}
		if newResource.Properties.Recipes != nil {
			errorPrefix := "recipe name(s) reserved for devRecipes for: "
			var errorRecipes string
			// validate that if the input recipe is updating an existing dev recipe with a different templatepath
			// if the input recipe has the same name as that of the dev recipe but different templatepath return an error
			for k, v := range newResource.Properties.Recipes {
				if val, ok := devRecipes[k]; ok && val.TemplatePath != v.TemplatePath {
					if errorRecipes != "" {
						errorRecipes += ", "
					}
					errorRecipes += fmt.Sprintf("recipe with name %s (linkType %s and templatePath %s)", k, v.LinkType, v.TemplatePath)
				}
			}
			if errorRecipes != "" {
				return nil, fmt.Errorf(errorPrefix + errorRecipes)
			}
		} else {
			newResource.Properties.Recipes = map[string]datamodel.EnvironmentRecipeProperties{}
		}
		maps.Copy(newResource.Properties.Recipes, devRecipes)
	}

	// Create Query filter to query kubernetes namespace used by the other environment resources.
	namespace := newResource.Properties.Compute.KubernetesCompute.Namespace
	result, err := util.FindResources(ctx, serviceCtx.ResourceID.RootScope(), serviceCtx.ResourceID.Type(), "properties.compute.kubernetes.namespace", namespace, e.StorageClient())
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
		if old == nil || env.ID != old.ID {
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

func getDevRecipes(ctx context.Context) (map[string]datamodel.EnvironmentRecipeProperties, error) {
	recipes := map[string]datamodel.EnvironmentRecipeProperties{}

	logger := ucplog.FromContext(ctx)
	reg, err := remote.NewRegistry(DevRecipesACRPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create client to registry %s -  %s", DevRecipesACRPath, err.Error())
	}

	// if repository has the correct path it should look like: <acrPath>/recipes/<linkType>/<provider>
	err = reg.Repositories(ctx, "", func(repos []string) error {
		for _, repo := range repos {
			link, provider := parseRepoPathForMetadata(repo)
			if link != "" && provider != "" {
				if slices.Contains(supportedProviders(), provider) {
					var name string
					var linkType string
					// TODO: this needs to metadata driven per-recipe so we don't have to maintain a lookup
					// table.
					switch link {
					case "mongodatabases":
						name = "mongo" + "-" + provider
						linkType = linkrp.MongoDatabasesResourceType
					case "rediscaches":
						name = "redis" + "-" + provider
						linkType = linkrp.RedisCachesResourceType
					default:
						continue
					}
					repoPath := DevRecipesACRPath + "/" + repo
					repoClient, err := remote.NewRepository(repoPath)
					if err != nil {
						return fmt.Errorf("failed to create client to repository %s -  %s", repoPath, err.Error())
					}

					// for a given repository, list the tags and identify what the latest version of the repo is
					// for now, only the latest verision of the repo is linked to the environment
					err = repoClient.Tags(ctx, "", func(tags []string) error {
						version, err := findHighestVersion(tags)
						if err != nil {
							return fmt.Errorf("error occurred while finding highest version for repo %s - %s", repoPath, err.Error())
						}
						recipes[name] = datamodel.EnvironmentRecipeProperties{
							LinkType:     linkType,
							TemplatePath: repoPath + ":" + fmt.Sprintf("%.1f", version),
						}
						return nil
					})
					if err != nil {
						return fmt.Errorf("failed to list tags for repository %s -  %s", repoPath, err.Error())
					}
				}
			}
		}

		logger.Info(fmt.Sprintf("pulled %d dev recipes", len(recipes)))

		// This function never returns an error as we currently silently continue on any repositories that don't have the path pattern specified.
		// It has a definition that specifies an error is returned to match the definition defined by reg.Repositories.
		// TODO: Add metrics here to identify how long this takes. Long-term, we should ensure the registry only has recipes. #4440
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list recipes available in registry at path  %s -  %s", DevRecipesACRPath, err.Error())
	}

	return recipes, nil
}

func parseRepoPathForMetadata(repo string) (link, provider string) {
	if strings.HasPrefix(repo, "recipes/") {
		recipePath := strings.Split(repo, "recipes/")[1]
		if strings.Count(recipePath, "/") == 1 {
			link, provider := strings.Split(recipePath, "/")[0], strings.Split(recipePath, "/")[1]
			return link, provider
		}
	}

	return link, provider
}

func findHighestVersion(versions []string) (latest float64, err error) {
	for _, version := range versions {
		f, err := strconv.ParseFloat(version, 32)
		if err != nil {
			return 0.0, fmt.Errorf("unable to convert tag %s into valid version.", version)
		}

		if f > latest {
			latest = f
		}
	}

	return latest, nil
}
