// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package utils

// GenerateEnvUrl Returns the URL string for an environment based on its subscriptionID and resourceGroup.
// Uses environment kind to determine how which kind-specific function should build the URL string.
func GenerateEnvUrl(kind, subscriptionID string, resourceGroup string) string {
	envUrl := ""
	if kind == "azure" {
		envUrl = generateEnvUrlAzure(subscriptionID, resourceGroup)
	} else {
		envUrl = "Env URL unknown."
	}

	return envUrl
}

// generateEnvUrlAzure Returns Returns the URL string for an Azure environment.
func generateEnvUrlAzure(subscriptionID string, resourceGroup string) string {

	envUrl := "https://ms.portal.azure.com/#@microsoft.onmicrosoft.com/resource/subscriptions/" +
		subscriptionID + "/resourceGroups/" + resourceGroup + "/overview"

	return envUrl
}
