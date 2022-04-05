// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armrpcv1

// OperationList is the object that we need to return for calls to list all available operations.
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
type OperationList struct {
	Value []Operation `json:"value"`
}

// Operation represents the struct which contains properties of an operation.
type Operation struct {
	Name         string                      `json:"name"`
	Display      *OperationDisplayProperties `json:"display"`
	Origin       string                      `json:"origin,omitempty"`
	IsDataAction bool                        `json:"isDataAction"`
}

// OperationDisplayProperties represents the struct which contains the display properties of an operation.
type OperationDisplayProperties struct {
	Description string `json:"description"`
	Operation   string `json:"operation"`
	Provider    string `json:"provider"`
	Resource    string `json:"resource"`
}
