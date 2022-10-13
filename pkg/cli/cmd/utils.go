// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

// CreateEnvAzureProvider forms the azure provider scope from the subscriptionID and resourceGroup
func CreateEnvAzureProvider(subscriptionID, resourceGroup string) corerp.Providers {
	providers := corerp.Providers{
		Azure: &corerp.ProvidersAzure{
			Scope: to.Ptr("/subscriptions/" + subscriptionID + "/resourceGroup/" + resourceGroup),
		},
	}
	return providers
}
