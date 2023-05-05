// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

const (
	ResourceTypeName  = "Applications.Core/environments"
	DevRecipesACRPath = "radius.azurecr.io"
	// User defined operation names
	OperationGetRecipeMetadata = "GETRECIPEMETADATA"
)

// supportedProviders returns the list of "known" providers we understand for dev recipes.
// this is used as a filter to exclude non-matching repositories from the dev recipes registry.
//
// There is no effect on the execution of the recipe.
func supportedProviders() []string {
	return []string{"aws", "azure", "kubernetes"}
}
