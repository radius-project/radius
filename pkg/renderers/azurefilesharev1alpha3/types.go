// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azurefilesharev1alpha3

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

type AzureFileShareProperties struct {
	Kind     string `json:"kind"`
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
