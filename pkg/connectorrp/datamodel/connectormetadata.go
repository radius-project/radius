// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"github.com/project-radius/radius/pkg/rp"
)

// ConnectorMetadata represents internal DataModel properties common to all connector types.
type ConnectorMetadata struct {
	// ComputedValues map is any resource values that will be needed for more operations.
	// For example; database name to generate secrets for cosmos DB.
	ComputedValues map[string]interface{} `json:"computedValues,omitempty"`

	// Stores action to retrieve secret values. For Azure, connectionstring is accessed through cosmos listConnectionString operation, if secrets are not provided as input
	SecretValues map[string]rp.SecretValueReference `json:"secretValues,omitempty"`
}
