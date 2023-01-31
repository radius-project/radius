// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	ResultPropertyName = "result"
)

// RecipeResponse is the output provided by deploying the recipe and reading its 'Result' output.
// It has the list of resourceId's that are deployed as well as the connection secrets and values.
type RecipeResponse struct {
	// Resources is the list of deployed resources.
	Resources []string `json:"resources"`

	// Secrets is a map of secret values.
	Secrets map[string]any `json:"secrets"`

	// Values is the map of connection values (non-secret).
	Values map[string]any `json:"values"`
}

// CreateRecipeContextParameter creates the context parameter for the recipe with the link, environment and application info
func CreateRecipeContextParameter(resourceID, environmentID, environmentNamespace, applicationID, applicationNamespace string) (*linkrp.RecipeContext, error) {
	recipeContext := linkrp.RecipeContext{}

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
func createRecipeParameters(devParams, operatorParams map[string]any, isCxtSet bool, recipeContext *linkrp.RecipeContext) map[string]any {
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

// prepareRecipeResponse populates the recipe response from parsing the deployment output 'result' object and the
// resources created by the template.
func prepareRecipeResponse(outputs any, resources []*armresources.ResourceReference) (RecipeResponse, error) {
	// We populate the recipe response from the 'result' output (if set)
	// and the resources created by the template.
	//
	// Note that there are two ways a resource can be returned:
	// - Implicitly when it is created in the template (it will be in 'resources').
	// - Explicitly as part of the 'result' output.
	//
	// The latter is needed because non-ARM and non-UCP resources are not returned as part of the implicit 'resources'
	// collection. For us this mostly means Kubernetes resources - the user has to be explicit.
	recipeResponse := RecipeResponse{}

	out, ok := outputs.(map[string]any)
	if ok {
		recipeOutput, ok := out[ResultPropertyName].(map[string]any)
		if ok {
			output, ok := recipeOutput["value"].(map[string]any)
			if ok {
				b, err := json.Marshal(&output)
				if err != nil {
					return RecipeResponse{}, err
				}

				// Using a decoder to block unknown fields.
				decoder := json.NewDecoder(bytes.NewBuffer(b))
				decoder.DisallowUnknownFields()
				err = decoder.Decode(&recipeResponse)
				if err != nil {
					return RecipeResponse{}, err
				}
			}
		}
	}

	// process the 'resources' created by the template
	for _, id := range resources {
		recipeResponse.Resources = append(recipeResponse.Resources, *id.ID)
	}

	// Make sure our maps are non-nil (it's just friendly).
	if recipeResponse.Secrets == nil {
		recipeResponse.Secrets = map[string]any{}
	}
	if recipeResponse.Values == nil {
		recipeResponse.Values = map[string]any{}
	}

	return recipeResponse, nil
}
