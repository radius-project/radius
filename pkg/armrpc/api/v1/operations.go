/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

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
