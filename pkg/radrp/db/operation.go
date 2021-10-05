// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package db

import (
	"time"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/armerrors"
)

type OperationKind string

const (
	OperationKindDelete OperationKind = "Delete"
	OperationKindUpdate OperationKind = "Update"
)

// See: https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/Addendum.md#asynchronous-operations
type Operation struct {
	ID            string        `bson:"id"`
	Name          string        `bson:"name"`
	Status        string        `bson:"status"`
	OperationKind OperationKind `bson:"operationKind"`

	// These should be in ISO8601 format
	StartTime string `bson:"startTime"`
	EndTime   string `bson:"endTime"`

	PercentComplete float64                 `bson:"percentComplete"`
	Properties      map[string]interface{}  `bson:"properties,omitempty"`
	Error           *armerrors.ErrorDetails `bson:"error"`
}

func NewOperation(id azresources.ResourceID, kind OperationKind, status string) Operation {
	return Operation{
		ID:            id.ID,
		Name:          id.Name(),
		Status:        status,
		OperationKind: kind,

		StartTime:       time.Now().UTC().Format(time.RFC3339),
		PercentComplete: 0,
	}
}
