// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

const (
	ResourceTypeName  = "Applications.Core/environments"
	DevRecipesACRPath = "radiusdev.azurecr.io"

	// User defined operation names
	OperationGetRecipeMetadata = "GETRECIPEMETADATA"
)

func supportedProviders() []string {
	return []string{"azure"}
}
