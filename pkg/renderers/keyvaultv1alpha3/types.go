// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package keyvaultv1alpha3

import (
	"github.com/project-radius/radius/pkg/azure/azresources"
)

const ResourceType = "azure.com.KeyVault"

var KeyVaultResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.KeyVaultVaults,
			Name: "*",
		},
	},
}
