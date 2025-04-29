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
	"strings"

	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	msg_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller"
	recipe_types "github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/version"
	"oras.land/oras-go/v2/registry/remote"
)

const (
	// RecipeRepositoryPrefix is the prefix for the repository path.
	DefaultRecipeRepositoryPrefix = "ghcr.io/radius-project/recipes/local-dev/"
)

type DevRecipe struct {
	// NormalizedName is the normalized name of the recipe.
	//
	// For example, "mongodatabases".
	NormalizedName string

	// ResourceType is the resource type of the recipe.
	//
	// For example, "Applications.Datastores/mongoDatabases".
	ResourceType string

	// RepoPath is the repository path of the recipe.
	//
	// For example, "ghcr.io/radius-project/recipes/local-dev/mongodatabases".
	RepoPath string
}

// AvailableDevRecipes returns the list of available dev recipes.
// If registryOverride is provided, it will use that as the base registry URL instead of the default.
func AvailableDevRecipes(registryOverride string) []DevRecipe {
	repoPrefix := DefaultRecipeRepositoryPrefix
	if registryOverride != "" {
		if !strings.HasSuffix(registryOverride, "/") {
			registryOverride += "/"
		}
		repoPrefix = registryOverride
	}

	return []DevRecipe{
		{
			"mongodatabases",
			ds_ctrl.MongoDatabasesResourceType,
			repoPrefix + "mongodatabases",
		},
		{
			"rediscaches",
			ds_ctrl.RedisCachesResourceType,
			repoPrefix + "rediscaches",
		},
		{
			"sqldatabases",
			ds_ctrl.SqlDatabasesResourceType,
			repoPrefix + "sqldatabases",
		},
		{
			"rabbitmqqueues",
			msg_ctrl.RabbitMQQueuesResourceType,
			repoPrefix + "rabbitmqqueues",
		},
		{
			"pubsubbrokers",
			dapr_ctrl.DaprPubSubBrokersResourceType,
			repoPrefix + "pubsubbrokers",
		},
		{
			"secretstores",
			dapr_ctrl.DaprSecretStoresResourceType,
			repoPrefix + "secretstores",
		},
		{
			"statestores",
			dapr_ctrl.DaprStateStoresResourceType,
			repoPrefix + "statestores",
		},
	}
}

//go:generate mockgen -typed -destination=./mock_devrecipeclient.go -package=radinit -self_package github.com/radius-project/radius/pkg/cli/cmd/radinit github.com/radius-project/radius/pkg/cli/cmd/radinit DevRecipeClient
type DevRecipeClient interface {
	GetDevRecipes(ctx context.Context, registryOverride string) (map[string]map[string]corerp.RecipePropertiesClassification, error)
}

type devRecipeClient struct {
}

// NewDevRecipeClient creates a new DevRecipeClient object and returns it.
func NewDevRecipeClient() DevRecipeClient {
	return &devRecipeClient{}
}

// GetDevRecipes is a function that queries a registry for recipes with a specific tag and returns a map of recipes.
// If registryOverride is provided, it will use that as the base registry URL instead of the default.
// If an error occurs, an error is returned.
func (drc *devRecipeClient) GetDevRecipes(ctx context.Context, registryOverride string) (map[string]map[string]corerp.RecipePropertiesClassification, error) {
	// The tag will be the major.minor version of the release.
	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}

	// Get available recipes with the registry override if provided
	recipes := AvailableDevRecipes(registryOverride)

	validDevRecipes := map[string]map[string]corerp.RecipePropertiesClassification{}
	for _, devRecipe := range recipes {
		repo, err := remote.NewRepository(devRecipe.RepoPath)
		if err != nil {
			continue
		}

		// The descriptor and the ReadCloser that are returned by FetchReference are not used.
		// If the tag does not exist, Not Found error is returned from the FetchReference function.
		_, _, err = repo.FetchReference(ctx, tag)
		if err == nil {
			validDevRecipes[devRecipe.ResourceType] = getRecipeProperties(devRecipe, tag)
		}
	}

	return validDevRecipes, nil
}

// getRecipeProperties returns the recipe properties for a specific recipe.
func getRecipeProperties(devRecipe DevRecipe, tag string) map[string]corerp.RecipePropertiesClassification {
	recipeName := "default"

	return map[string]corerp.RecipePropertiesClassification{
		recipeName: &corerp.BicepRecipeProperties{
			TemplateKind: to.Ptr(recipe_types.TemplateKindBicep),
			TemplatePath: to.Ptr(devRecipe.RepoPath + ":" + tag),
		},
	}
}
