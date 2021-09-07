// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbmongov1alpha1

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const (
	Kind          = "azure.com/CosmosDBMongo@v1alpha1"
	BindingCosmos = "cosmos"
	BindingMongo  = "mongo"
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

// CosmosDBMongoComponent definition of CosmosDBMongo component
type CosmosDBMongoComponent struct {
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Config   CosmosDBMongoConfig      `json:"config,omitempty"`
	Run      map[string]interface{}   `json:"run,omitempty"`
	Uses     []map[string]interface{} `json:"uses,omitempty"`
	Bindings []map[string]interface{} `json:"bindings,omitempty"`
	Traits   []map[string]interface{} `json:"traits,omitempty"`
}

// CosmosDBMongoConfig defintion of the config section
type CosmosDBMongoConfig struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
