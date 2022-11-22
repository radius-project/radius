// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

const (
	ResourceTypeName  = "Applications.Core/environments"
	DevRecipesACRPath = "radiusdev.azurecr.io"
)

func supportedProviders() []string {
	return []string{"azure"}
}
