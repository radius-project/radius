// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha3

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const ResourceType = "azure.com.KeyVaultComponent"

var KeyVaultResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.KeyVaultVaults,
			Name: "*",
		},
	},
}
