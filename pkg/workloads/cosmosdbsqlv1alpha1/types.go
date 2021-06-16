// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbsqlv1alpha1

import "github.com/Azure/radius/pkg/radrp/resources"

const (
	Kind = "azure.com/CosmosDBSQL@v1alpha1"
)

var SQLResourceType = resources.KnownType{
	Types: []resources.ResourceType{
		{
			Type: "Microsoft.DocumentDB/databaseAccounts",
			Name: "*",
		},
		{
			Type: "sqlDatabases",
			Name: "*",
		},
	},
}

// CosmosDBSQLComponent definition of CosmosDBSQL component
type CosmosDBSQLComponent struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    CosmosDBSQLConfig        `json:"config,omitempty"`
	Run       map[string]interface{}   `json:"run,omitempty"`
	DependsOn []map[string]interface{} `json:"dependson,omitempty"`
	Provides  []map[string]interface{} `json:"provides,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
}

// CosmosDBSQLConfig defintion of the config section
type CosmosDBSQLConfig struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
