// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import "github.com/Azure/radius/pkg/azure/azresources"

const (
	ResourceType          = "mongodb.com.MongoDBComponent"
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

type MongoDBComponentProperties struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
