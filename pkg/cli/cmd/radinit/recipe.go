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
	ext_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/extenders"
	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	msg_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller"
	recipe_types "github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/version"
)

const (
	DevRecipesRegistry = "ghcr.io"
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
	// reg, err := remote.NewRegistry(DevRecipesRegistry)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to create client to registry %s -  %s", DevRecipesRegistry, err.Error())
	// }

	// The tag will be the major.minor version of the release.
	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}

	// Temporary solution to get all repositories.
	// The issue is that if RepositoryListPageSize is not specified the default is 100.
	// We have 104 repositories in the registry as of 12 Oct 2023. That is why processRepositories
	// function was being called twice and the second call was overwriting all the recipes.
	// TODO: Remove this once we have a better solution.
	// reg.RepositoryListPageSize = 1000

	// recipes := map[string]map[string]corerp.RecipePropertiesClassification{}

	// // if repository has the correct path it should look like: <registryPath>/recipes/<category>/<type>:<tag>
	// // Ex: ghcr.io/radius-project/recipes/local-dev/rediscaches:0.20
	// // The start parameter is set to "radius-rp" because our recipes are after that repository.
	// err = reg.Repositories(ctx, "radius-project", func(repos []string) error {
	// 	// validRepos will contain the repositories that have the requested tag.
	// 	validRepos := []string{}
	// 	for _, repo := range repos {
	// 		r, err := reg.Repository(ctx, repo)
	// 		if err != nil {
	// 			continue
	// 		}

	// 		tagExists := false
	// 		err = r.Tags(ctx, "", func(tags []string) error {
	// 			for _, t := range tags {
	// 				if t == tag {
	// 					tagExists = true
	// 					break
	// 				}
	// 			}
	// 			return nil
	// 		})
	// 		if err != nil {
	// 			continue
	// 		}

	// 		if tagExists {
	// 			validRepos = append(validRepos, repo)
	// 		}
	// 	}

	// 	recipes = processRepositories(validRepos, tag)
	// 	return nil
	// })
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to list recipes available in registry at path  %s -  %s", DevRecipesRegistry, err.Error())
	// }

	recipes := map[string]map[string]corerp.RecipePropertiesClassification{
		"Applications.Datastores/sqlDatabases": {
			"default": &corerp.BicepRecipeProperties{
				TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
				TemplatePath: to.Ptr(fmt.Sprintf("ghcr.io/radius-project/recipes/local-dev/sqldatabases:%s", tag)),
			},
		},
		"Applications.Messaging/rabbitMQQueues": {
			"default": &corerp.BicepRecipeProperties{
				TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
				TemplatePath: to.Ptr(fmt.Sprintf("ghcr.io/radius-project/recipes/local-dev/rabbitmqqueues:%s", tag)),
			},
		},
		"Applications.Dapr/pubSubBrokers": {
			"default": &corerp.BicepRecipeProperties{
				TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
				TemplatePath: to.Ptr(fmt.Sprintf("ghcr.io/radius-project/recipes/local-dev/pubsubbrokers:%s", tag)),
			},
		},
		"Applications.Dapr/secretStores": {
			"default": &corerp.BicepRecipeProperties{
				TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
				TemplatePath: to.Ptr(fmt.Sprintf("ghcr.io/radius-project/recipes/local-dev/secretStores:%s", tag)),
			},
		},
		"Applications.Dapr/stateStores": {
			"default": &corerp.BicepRecipeProperties{
				TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
				TemplatePath: to.Ptr(fmt.Sprintf("ghcr.io/radius-project/recipes/local-dev/stateStores:%s", tag)),
			},
		},
		"Applications.Datastores/mongoDatabases": {
			"default": &corerp.BicepRecipeProperties{
				TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
				TemplatePath: to.Ptr(fmt.Sprintf("ghcr.io/radius-project/recipes/local-dev/mongodatabases:%s", tag)),
			},
		},
		"Applications.Datastores/redisCaches": {
			"default": &corerp.BicepRecipeProperties{
				TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
				TemplatePath: to.Ptr(fmt.Sprintf("ghcr.io/radius-project/recipes/local-dev/rediscaches:%s", tag)),
			},
		},
	}

	return recipes, nil
}

// processRepositories processes the repositories and returns the recipes.
// func processRepositories(repos []string, tag string) map[string]map[string]corerp.RecipePropertiesClassification {
// 	recipes := map[string]map[string]corerp.RecipePropertiesClassification{}

// 	// We are using the default recipe.
// 	name := "default"

// 	for _, repo := range repos {
// 		// Skip dev environment recipes.
// 		// dev repositories is in the form of ghcr.io/radius-project/dev/recipes/local-dev/secretstores:latest
// 		// We should skip the dev repositories.
// 		if isDevRepository(repo) {
// 			continue
// 		}

// 		resourceType := getResourceTypeFromPath(repo)
// 		// If the resource type is empty, it means we don't support the repository.
// 		if resourceType == "" {
// 			continue
// 		}

// 		portableResourceType := getPortableResourceType(resourceType)
// 		// If the PortableResource type is empty, it means we don't support the resource type.
// 		if portableResourceType == "" {
// 			continue
// 		}

// 		repoPath := DevRecipesRegistry + "/" + repo

// 		recipes[portableResourceType] = map[string]corerp.RecipePropertiesClassification{
// 			name: &corerp.BicepRecipeProperties{
// 				TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
// 				TemplatePath: to.Ptr(repoPath + ":" + tag),
// 			},
// 		}
// 	}

// 	return recipes
// }

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
		return ds_ctrl.MongoDatabasesResourceType
	case "rediscaches":
		return ds_ctrl.RedisCachesResourceType
	case "sqldatabases":
		return ds_ctrl.SqlDatabasesResourceType
	case "rabbitmqqueues":
		return msg_ctrl.RabbitMQQueuesResourceType
	case "pubsubbrokers":
		return dapr_ctrl.DaprPubSubBrokersResourceType
	case "secretstores":
		return dapr_ctrl.DaprSecretStoresResourceType
	case "statestores":
		return dapr_ctrl.DaprStateStoresResourceType
	case "extenders":
		return ext_ctrl.ResourceTypeName
	default:
		return ""
	}
}

func isDevRepository(repo string) bool {
	_, found := strings.CutPrefix(repo, "dev/")
	return found
}
