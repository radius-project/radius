// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbmongov1alpha3

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const (
	ResourceType          = "azure.com.CosmosDBMongoComponent"
	ConnectionStringValue = "connectionString"
	DatabaseValue         = "database"
)

var MongoResourceType = azresources.KnownType{
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

type CosmosDBMongoComponentProperties struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
