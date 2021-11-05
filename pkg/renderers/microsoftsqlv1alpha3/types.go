// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package microsoftsqlv1alpha3

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const (
	ResourceType          = "microsoft.com.SQLComponent"
	ConnectionStringValue = "connectionString"
)

var SQLResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.SqlServers,
			Name: "*",
		},
		{
			Type: azresources.SqlServersDatabases,
			Name: "*",
		},
	},
}
