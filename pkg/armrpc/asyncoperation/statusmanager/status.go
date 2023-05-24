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

package statusmanager

import (
	time "time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// Status is the datamodel for Async operation statuses.
type Status struct {
	v1.AsyncOperationStatus

	// LinkedResourceID is the resource id associated with operation status.
	LinkedResourceID string `json:"resourceID"`

	// Location represents the location of operationstatus.
	Location string `json:"location"`

	// RetryAfter is the value of the Retry-After header that will be used for async operations.
	RetryAfter time.Duration `json:"retryAfter"`

	// HomeTenantID is async operation caller's tenant id such as the value from x-ms-home-tenant-id header.
	HomeTenantID string `json:"clientTenantID,omitempty"`

	// ClientObjectID is async operation caller's client id such as the value from x-ms-client-object-id header.
	ClientObjectID string `json:"clientObjectID,omitempty"`

	// LastUpdatedTime represents the async operation last updated time.
	LastUpdatedTime time.Time `json:"lastUpdatedTime,omitempty"`
}
