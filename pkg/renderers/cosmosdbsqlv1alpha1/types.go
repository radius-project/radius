// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbsqlv1alpha1

import (
	"github.com/Azure/radius/pkg/azure/azresources"
)

const (
	Kind = "azure.com/CosmosDBSQL@v1alpha1"
)

var SQLResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.DocumentDBDatabaseAccounts,
			Name: "*",
		},
		{
			Type: azresources.DocumentDBDatabaseAccountsSQLDatabases,
			Name: "*",
		},
	},
}

// CosmosDBSQLComponent definition of CosmosDBSQL component
type CosmosDBSQLComponent struct {
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Config   CosmosDBSQLConfig        `json:"config,omitempty"`
	Run      map[string]interface{}   `json:"run,omitempty"`
	Uses     []map[string]interface{} `json:"uses,omitempty"`
	Bindings []map[string]interface{} `json:"bindings,omitempty"`
	Traits   []map[string]interface{} `json:"traits,omitempty"`
}

// CosmosDBSQLConfig defintion of the config section
type CosmosDBSQLConfig struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
