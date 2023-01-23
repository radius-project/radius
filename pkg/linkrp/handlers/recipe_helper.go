// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"fmt"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// CreateRecipeContextParameter creates the context parameter for the recipe with the link, environment and application info
func CreateRecipeContextParameter(resourceID, environmentID, environmentNamespace, applicationID, applicationNamespace string) (*datamodel.RecipeContext, error) {
	linkContext := datamodel.RecipeContext{}

	parsedLink, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resourceID : %q while building the context parameter %q", resourceID, err.Error())
	}
	linkContext.Resource.ID = resourceID
	linkContext.Resource.Name = parsedLink.Name()
	linkContext.Resource.Type = parsedLink.Type()

	linkContext.Environment.ID = environmentID
	parsedEnv, err := resources.ParseResource(environmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse environmentID : %q while building the context parameter %q", environmentID, err.Error())
	}
	linkContext.Environment.Name = parsedEnv.Name()
	linkContext.Runtime.Kubernetes.Namespace = environmentNamespace

	if applicationID != "" {
		parsedApp, err := resources.ParseResource(applicationID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse applicationID : %q while building the context parameter %q", applicationID, err.Error())
		}
		linkContext.Application.ID = applicationID
		linkContext.Application.Name = parsedApp.Name()
		linkContext.Runtime.Kubernetes.Namespace = applicationNamespace
	}
	return &linkContext, nil
}

// handleParameterConflict handles conflicts in parameters set by operator and developer
// In case of conflict the developer parameter takes precedence
func handleParameterConflict(devParams, operatorParams map[string]any) map[string]any {
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
	return parameters
}
