// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armrpcv1

// Operation represents the struct which contains properties of an operation.
// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
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
