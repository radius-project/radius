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

// see : https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/common-api-details.md#error-response-content

// ErrorResponse represents an error HTTP response as defined by the ARM API.
type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

// ErrorDetails represents an error as defined by the ARM API.
type ErrorDetails struct {
	Code           string                `json:"code"`
	Message        string                `json:"message"`
	Target         string                `json:"target,omitempty"`
	AdditionalInfo []ErrorAdditionalInfo `json:"additionalInfo,omitempty"`
	Details        []ErrorDetails        `json:"details,omitempty"`
}

// ErrorAdditionalInfo represents abritrary additional information as part of an error as defined by the ARM API.
type ErrorAdditionalInfo struct {
	Type string         `json:"type"`
	Info map[string]any `json:"info"`
}
