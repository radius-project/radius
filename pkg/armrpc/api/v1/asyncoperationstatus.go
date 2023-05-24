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

import (
	"time"
)

// AsyncOperationStatus represents an OperationStatus resource.
type AsyncOperationStatus struct {
	// Id represents the async operation id.
	ID string `json:"id,omitempty"`

	// Name represents the async operation name and is usually set to the async operation id.
	Name string `json:"name,omitempty"`

	// Status represents the provisioning state of the resource.
	Status ProvisioningState `json:"status,omitempty"`

	// StartTime represents the async operation start time.
	StartTime time.Time `json:"startTime,omitempty"`

	// EndTime represents the async operation end time.
	EndTime *time.Time `json:"endTime,omitempty"`

	// Error represents the error occured during provisioning.
	Error *ErrorDetails `json:"error,omitempty"`
}
