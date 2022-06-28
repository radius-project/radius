// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var AzureSQLResourceType = resources.KnownType{
	Types: []resources.TypeSegment{
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
