// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdocumentdbv1alpha1

const Kind = "azure.com/CosmosDocumentDb@v1alpha1"

// CosmosDocumentDbComponent is the definition of the container component
type CosmosDocumentDbComponent struct {
	Name      string                   `json:"name"`
	Kind      string                   `json:"kind"`
	Config    CosmosDocumentDbConfig   `json:"config,omitempty"`
	Run       map[string]interface{}   `json:"run,omitempty"`
	DependsOn []map[string]interface{} `json:"dependson,omitempty"`
	Provides  []map[string]interface{} `json:"provides,omitempty"`
	Traits    []map[string]interface{} `json:"traits,omitempty"`
}

// CosmosDocumentDbConfig is the defintion of the config section
type CosmosDocumentDbConfig struct {
	Managed bool `json:"managed"`
}
