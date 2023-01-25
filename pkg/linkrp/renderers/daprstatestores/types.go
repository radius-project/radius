// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var StorageAccountResourceType = resources.KnownType{
	Types: []resources.TypeSegment{
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
