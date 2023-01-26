// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

// CreateEnvProviders forms the azure provider scope from the subscriptionID and resourceGroup
func CreateEnvProviders(subscriptionID, resourceGroup string, accountID string, region string) corerp.Providers {
	var azureProvider *corerp.ProvidersAzure
	var awsProvider *corerp.ProvidersAws
	if subscriptionID != "" && resourceGroup != "" {
		azureProvider = &corerp.ProvidersAzure{
			Scope: to.Ptr("/subscriptions/" + subscriptionID + "/resourceGroups/" + resourceGroup),
		}
	}
	if accountID != "" && region != "" {
		awsProvider = &corerp.ProvidersAws{
			Scope: to.Ptr("/accounts/" + accountID + "/regions/" + region),
		}
	}

	providers := corerp.Providers{
		Azure: azureProvider,
		Aws:   awsProvider,
	}
	return providers
}

func GetNamespace(envResource corerp.EnvironmentResource) string {
	switch v := envResource.Properties.Compute.(type) {
	case *corerp.KubernetesCompute:
		return *v.Namespace
	}
	return ""
}
