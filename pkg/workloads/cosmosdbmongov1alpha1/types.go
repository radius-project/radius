// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbmongov1alpha1

const (
	Kind = "azure.com/CosmosDBMongo@v1alpha1"
)

// CosmosDBMongoComponent definition of CosmosDBMongo component
type CosmosDBMongoComponent struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    CosmosDBMongoConfig      `json:"config,omitempty"`
	Run       map[string]interface{}   `json:"run,omitempty"`
	DependsOn []map[string]interface{} `json:"dependson,omitempty"`
	Provides  []map[string]interface{} `json:"provides,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
}

// CosmosDBMongoConfig defintion of the config section
type CosmosDBMongoConfig struct {
	Managed bool `json:"managed"`
}
