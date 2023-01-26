// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"fmt"

	dockerParser "github.com/novln/docker-parser"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// CreateRecipeContextParameter creates the context parameter for the recipe with the link, environment and application info
func CreateRecipeContextParameter(resourceID, environmentID, environmentNamespace, applicationID, applicationNamespace string) (*datamodel.RecipeContext, error) {
	recipeContext := datamodel.RecipeContext{}

	parsedLink, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resourceID: %q while building the recipe context parameter %w", resourceID, err)
	}
	recipeContext.Resource.ID = resourceID
	recipeContext.Resource.Name = parsedLink.Name()
	recipeContext.Resource.Type = parsedLink.Type()

	recipeContext.Environment.ID = environmentID
	parsedEnv, err := resources.ParseResource(environmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse environmentID: %q while building the recipe context parameter %w", environmentID, err)
	}
	recipeContext.Environment.Name = parsedEnv.Name()
	recipeContext.Runtime.Kubernetes.Namespace = environmentNamespace
	recipeContext.Runtime.Kubernetes.EnvironmentNamespace = environmentNamespace

	if applicationID != "" {
		parsedApp, err := resources.ParseResource(applicationID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse applicationID :%q while building the recipe context parameter %w", applicationID, err)
		}
		recipeContext.Application.ID = applicationID
		recipeContext.Application.Name = parsedApp.Name()
		recipeContext.Runtime.Kubernetes.Namespace = applicationNamespace
	}
	return &recipeContext, nil
}

// createRecipeParameters creates the parameters to be passed for recipe deployment after handling conflicts in parameters set by operator and developer.
// In case of conflict the developer parameter takes precedence. If recipe has context parameter defined adds the context information to the parameters list
func createRecipeParameters(devParams, operatorParams map[string]any, isCxtSet bool, recipeContext *datamodel.RecipeContext) map[string]any {
	parameters := map[string]any{}
	for k, v := range operatorParams {
		if _, ok := devParams[k]; !ok {
			devParams[k] = v
		}
	}
	for k, v := range devParams {
		parameters[k] = map[string]any{
			"value": v,
		}
	}
	if isCxtSet {
		parameters["context"] = map[string]any{
			"value": *recipeContext,
		}
	}
	return parameters
}

func parseTemplatePath(templatePath string) (repository string, tag string, err error) {
	reference, err := dockerParser.Parse(templatePath)
	if err != nil {
		return "", "", err
	}
	repository = reference.Repository()
	tag = reference.Tag()
	return
}
