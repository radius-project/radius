// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha3

import "github.com/Azure/radius/pkg/azure/azresources"

const (
	StorageKeyValue    = "azurestorageaccountkey"
	StorageAccountName = "azurestorageaccountname"
	ResourceType       = "Volume"
)

var AzureFileShareResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.StorageStorageAccounts,
			Name: "*",
		},
		{
			Type: azresources.AzureFileShareFileServices,
			Name: "*",
		},
		{
			Type: azresources.AzureFileShareShares,
			Name: "*",
		},
	},
}
