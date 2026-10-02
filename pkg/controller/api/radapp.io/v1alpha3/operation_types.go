/*
Copyright 2023.

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

package v1alpha3

import "net/http"

// ResourceOperation describes the status of an in-progress provisioning operation.
type ResourceOperation struct {
	// ResumeToken is a token that can be used to resume an in-progress provisioning operation.
	ResumeToken string `json:"resumeToken,omitempty"`

	// OperationKind describes the type of operation being performed.
	OperationKind OperationKind `json:"operationKind,omitempty"`
}

// OperationKind is the type of operation being performed.
type OperationKind string

const (
	// OperationKindPut is a PUT (create or update) operation.
	OperationKindPut = http.MethodPut

	// OperationKindDelete is a DELETE operation.
	OperationKindDelete = http.MethodDelete
)
