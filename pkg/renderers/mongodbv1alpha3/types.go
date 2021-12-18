// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import "github.com/Azure/radius/pkg/azure/azresources"

const (
	ResourceType          = "mongo.com.MongoDatabase"
	ConnectionStringValue = "connectionString"
	DatabaseValue         = "database"
)

var CosmosMongoResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
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
