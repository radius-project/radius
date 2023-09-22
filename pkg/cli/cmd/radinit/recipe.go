/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package radinit

import (
	"context"
	"fmt"
	"strings"

	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/portableresources"
	recipe_types "github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/version"
	"oras.land/oras-go/v2/registry/remote"
)

const (
	DevRecipesRegistry = "radius.azurecr.io"
)

//go:generate mockgen -destination=./mock_devrecipeclient.go -package=radinit -self_package github.com/radius-project/radius/pkg/cli/cmd/radinit github.com/radius-project/radius/pkg/cli/cmd/radinit DevRecipeClient
type DevRecipeClient interface {
	GetDevRecipes(ctx context.Context) (map[string]map[string]corerp.RecipePropertiesClassification, error)
}

type devRecipeClient struct {
}

// NewDevRecipeClient creates a new DevRecipeClient object and returns it.
func NewDevRecipeClient() DevRecipeClient {
	return &devRecipeClient{}
}

// GetDevRecipes is a function that queries a registry for recipes with a specific tag and returns a map of recipes.
// If an error occurs, an error is returned.
func (drc *devRecipeClient) GetDevRecipes(ctx context.Context) (map[string]map[string]corerp.RecipePropertiesClassification, error) {
	reg, err := remote.NewRegistry(DevRecipesRegistry)
	if err != nil {
		return nil, fmt.Errorf("failed to create client to registry %s -  %s", DevRecipesRegistry, err.Error())
	}

	// The tag will be the major.minor version of the release.
	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}

	recipes := map[string]map[string]corerp.RecipePropertiesClassification{}

	// if repository has the correct path it should look like: <registryPath>/recipes/<category>/<type>:<tag>
	// Ex: radius.azurecr.io/recipes/local-dev/rediscaches:0.20
	err = reg.Repositories(ctx, "", func(repos []string) error {
		// validRepos will contain the repositories that have the requested tag.
		validRepos := []string{}
		for _, repo := range repos {
			r, err := reg.Repository(ctx, repo)
			if err != nil {
				continue
			}

			tagExists := false
			err = r.Tags(ctx, "", func(tags []string) error {
				for _, t := range tags {
					if t == tag {
						tagExists = true
						break
					}
				}
				return nil
			})
			if err != nil {
				continue
			}

			if tagExists {
				validRepos = append(validRepos, repo)
			}
		}

		recipes = processRepositories(validRepos, tag)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list recipes available in registry at path  %s -  %s", DevRecipesRegistry, err.Error())
	}

	return recipes, nil
}

// processRepositories processes the repositories and returns the recipes.
func processRepositories(repos []string, tag string) map[string]map[string]corerp.RecipePropertiesClassification {
	recipes := map[string]map[string]corerp.RecipePropertiesClassification{}

	// We are using the default recipe.
	name := "default"

	for _, repo := range repos {
		resourceType := getResourceTypeFromPath(repo)
		// If the resource type is empty, it means we don't support the repository.
		if resourceType == "" {
			continue
		}

		portableResourceType := getPortableResourceType(resourceType)
		// If the PortableResource type is empty, it means we don't support the resource type.
		if portableResourceType == "" {
			continue
		}

		repoPath := DevRecipesRegistry + "/" + repo

		recipes[portableResourceType] = map[string]corerp.RecipePropertiesClassification{
			name: &corerp.BicepRecipeProperties{
				TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
				TemplatePath: to.Ptr(repoPath + ":" + tag),
			},
		}
	}

	return recipes
}

// getResourceTypeFromPath parses the repository path to extract the resource type.
//
// Should be of the form: recipes/local-dev/<resourceType>
func getResourceTypeFromPath(repo string) (resourceType string) {
	_, after, found := strings.Cut(repo, "recipes/local-dev/")
	if !found || after == "" {
		return ""
	}

	if strings.Count(after, "/") == 0 {
		resourceType = strings.Split(after, "/")[0]
	}

	return resourceType
}

// getPortableResourceType returns the resource type for the given resource.
func getPortableResourceType(resourceType string) string {
	switch resourceType {
	case "mongodatabases":
		return portableresources.MongoDatabasesResourceType
	case "rediscaches":
		return portableresources.RedisCachesResourceType
	case "sqldatabases":
		return portableresources.SqlDatabasesResourceType
	case "rabbitmqqueues":
		return portableresources.RabbitMQQueuesResourceType
	case "pubsubbrokers":
		return portableresources.DaprPubSubBrokersResourceType
	case "secretstores":
		return portableresources.DaprSecretStoresResourceType
	case "statestores":
		return portableresources.DaprStateStoresResourceType
	case "extenders":
		return portableresources.ExtendersResourceType
	default:
		return ""
	}
}
