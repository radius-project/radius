// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radinit

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	"golang.org/x/exp/slices"
	"oras.land/oras-go/v2/registry/remote"
)

const (
	DevRecipesACRPath = "radius.azurecr.io"
)

// supportedProviders returns the list of "known" providers we understand for dev recipes.
// this is used as a filter to exclude non-matching repositories from the dev recipes registry.
//
// This is no effect on the execution of the recipe.
func supportedProviders() []string {
	return []string{"aws", "azure", "kubernetes"}
}

var getDevRecipes = func(ctx context.Context) (map[string]*corerp.EnvironmentRecipeProperties, error) {
	recipes := map[string]*corerp.EnvironmentRecipeProperties{}

	logger := ucplog.FromContextOrDiscard(ctx)
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
						recipes[name] = &corerp.EnvironmentRecipeProperties{
							LinkType:     to.Ptr(linkType),
							TemplatePath: to.Ptr(repoPath + ":" + fmt.Sprintf("%.1f", version)),
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
			return 0.0, fmt.Errorf("unable to convert tag %s into valid version", version)
		}

		if f > latest {
			latest = f
		}
	}

	return latest, nil
}
