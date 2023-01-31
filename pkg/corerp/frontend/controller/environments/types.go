// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

const (
	ResourceTypeName  = "Applications.Core/environments"
	DevRecipesACRPath = "radiusdev.azurecr.io"

	// User defined operation names
	OperationGetRecipeDetails = "GETRECIPEDETAILS"
)

func supportedProviders() []string {
	return []string{"azure"}
}
