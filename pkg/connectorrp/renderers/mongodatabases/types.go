// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	ResourceType = "Applications.Connector/mongoDatabases" // use controller.ResourceTypeName instead
)

var AzureCosmosMongoResourceType = resources.KnownType{
	Types: []resources.TypeSegment{
		{
			Type: azresources.DocumentDBDatabaseAccounts,
			Name: "*",
		},
		{
			Type: azresources.DocumentDBDatabaseAccountsMongodDBDatabases,
			Name: "*",
		},
	},
}
