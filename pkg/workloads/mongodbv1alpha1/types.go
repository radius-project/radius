// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha1

import "github.com/Azure/radius/pkg/workloads/cosmosdbmongov1alpha1"

const (
	Kind         = "mongodb.com/Mongo@v1alpha1"
	BindingMongo = cosmosdbmongov1alpha1.BindingMongo
)

type MongoDBComponent struct {
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Config   MongoDBConfig            `json:"config,omitempty"`
	Run      map[string]interface{}   `json:"run,omitempty"`
	Uses     []map[string]interface{} `json:"uses,omitempty"`
	Bindings []map[string]interface{} `json:"bindings,omitempty"`
	Traits   []map[string]interface{} `json:"traits,omitempty"`
}

type MongoDBConfig struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
