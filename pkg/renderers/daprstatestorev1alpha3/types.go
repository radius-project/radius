// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha3

import (
	"github.com/project-radius/radius/pkg/azure/azresources"
)

const (
	ResourceType = "dapr.io.StateStore"
)

var StorageAccountResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.StorageStorageAccounts,
			Name: "*",
		},
		{
			Type: azresources.StorageStorageTableServices,
			Name: "*",
		},
		{
			Type: azresources.StorageStorageAccountsTables,
			Name: "*",
		},
	},
}

type Properties struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
}
