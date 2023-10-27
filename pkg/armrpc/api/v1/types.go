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
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultRetryAfter is the default value in seconds for the Retry-After header. This value is used
	// to determine the polling frequency of the client for long-running operations. Consider setting
	// a smaller value like 5 seconds if your operations are expected to be fast.
	DefaultRetryAfter = "60"

	// DefaultRetryAfterDuration is the default value in time.Duration for the Retry-After header. This value is used
	// to determine the polling frequency of the client for long-running operations. Consider setting
	// a smaller value like 5 seconds if your operations are expected to be fast.
	DefaultRetryAfterDuration = 60 * time.Second
)

var (
	// ErrInvalidOperationType represents the error for invalid operation type string.
	ErrInvalidOperationType = errors.New("failed to parse operation type")
)

// OperationMethod is the ARM operation of resource.
type OperationMethod string

var operationMethodToHTTPMethod = map[OperationMethod]string{
	OperationPlaneScopeList: http.MethodGet,
	OperationList:           http.MethodGet,
	OperationGet:            http.MethodGet,
	OperationPut:            http.MethodPut,
	OperationPatch:          http.MethodPatch,
	OperationDelete:         http.MethodDelete,

	// ARM RPC specific operations.
	OperationPutSubscriptions: http.MethodPut,

	// Non-idempotent lifecycle operations.
	OperationGetImperative:    http.MethodPost,
	OperationPutImperative:    http.MethodPost,
	OperationDeleteImperative: http.MethodPost,

	// No defined HTTP method for proxying.
	OperationProxy: "",
}

// HTTPMethod returns HTTP method corresponding to the given OperationMethod, or POST if
// no corresponding method is found.
func (o OperationMethod) HTTPMethod() string {
	m, ok := operationMethodToHTTPMethod[o]
	if !ok {
		// ARM RPC defines CRUD_L operations of one resource type and the custom action should be defined as POST method.
		// For example, if we want to support `listSecret` API for mongodatabase, this API must be defined as POST method.
		// POST /subscriptions/{subId}/resourcegroups/{rg}/applications.datastores/mongodatabases/{mongo}/listSecret
		return http.MethodPost
	}
	return m
}

const (
	// Predefined Operation methods.
	OperationPlaneScopeList   OperationMethod = "LISTPLANESCOPE"
	OperationList             OperationMethod = "LIST"
	OperationGet              OperationMethod = "GET"
	OperationPut              OperationMethod = "PUT"
	OperationPatch            OperationMethod = "PATCH"
	OperationDelete           OperationMethod = "DELETE"
	OperationPutSubscriptions OperationMethod = "PUTSUBSCRIPTIONS"
	OperationPost             OperationMethod = "POST"

	// Imperative operation methods for non-idempotent lifecycle operations.
	// UCP extends the ARM resource lifecycle to support using POST for non-idempotent resource types.
	//
	// Normally, the resource URL must include the resource name, but some resource types disallow user-specified
	// names, so the URL cannot be constructed until after the resource is created.

	// OperationGetImperative is used for non-idempotent GET operations.
	OperationGetImperative OperationMethod = "GETIMPERATIVE"

	// OperationPutImperative is used for non-idempotent PUT operations.
	OperationPutImperative OperationMethod = "PUTIMPERATIVE"

	// OperationDeleteImperative is used for non-idempotent DELETE operations.
	OperationDeleteImperative OperationMethod = "DELETEIMPERATIVE"

	// OperationProxy is used for controllers that proxy the underlying request without classifying the type of operation.
	OperationProxy OperationMethod = "PROXY"

	Separator = "|"
)

// OperationType represents the operation type which includes resource type name and its method.
// OperationType is used as a route name in the frontend API server router. Each valid ARM RPC call should have
// its own operation type name. For Asynchronous API, the frontend API server queues the async operation
// request with this operation type. AsyncRequestProcessWorker parses the operation type from the message
// and run the corresponding async operation controller.
type OperationType struct {
	Type   string
	Method OperationMethod
}

// String returns the operation type string.
func (o OperationType) String() string {
	return strings.ToUpper(o.Type + Separator + string(o.Method))
}

// ParseOperationType parses OperationType from string.
func ParseOperationType(s string) (OperationType, bool) {
	p := strings.Split(s, Separator)
	if len(p) == 2 {
		return OperationType{
			Type:   strings.ToUpper(p[0]),
			Method: OperationMethod(strings.ToUpper(p[1])),
		}, true
	}
	return OperationType{}, false
}

// ProvisioningState is the state of resource.
type ProvisioningState string

const (
	ProvisioningStateNone         ProvisioningState = "None"
	ProvisioningStateUpdating     ProvisioningState = "Updating"
	ProvisioningStateDeleting     ProvisioningState = "Deleting"
	ProvisioningStateAccepted     ProvisioningState = "Accepted"
	ProvisioningStateSucceeded    ProvisioningState = "Succeeded"
	ProvisioningStateProvisioning ProvisioningState = "Provisioning"
	ProvisioningStateProvisioned  ProvisioningState = "Provisioned"
	ProvisioningStateFailed       ProvisioningState = "Failed"
	ProvisioningStateCanceled     ProvisioningState = "Canceled"
	ProvisioningStateUndefined    ProvisioningState = "Undefined"
)

// IsTerminal returns true if given Provisioning State is in a terminal state.
func (state ProvisioningState) IsTerminal() bool {
	// If state is empty, it is the resource created by synchronous API and treated as a terminal state.
	return state == ProvisioningStateSucceeded || state == ProvisioningStateFailed || state == ProvisioningStateCanceled || state == ""
}

// TrackedResource represents the common tracked resource.
type TrackedResource struct {
	// ID is the fully qualified resource ID for the resource.
	ID string `json:"id"`
	// Name is the resource name.
	Name string `json:"name"`
	// Type is the resource type.
	Type string `json:"type"`
	// Location is the geo-location where resource is located.
	Location string `json:"location"`
	// Tags is the resource tags.
	Tags map[string]string `json:"tags,omitempty"`
}

// InternalMetadata represents internal DataModel specific metadata.
type InternalMetadata struct {
	// TenantID is the tenant id of the resource.
	TenantID string `json:"tenantId"`
	// CreatedAPIVersion is an api-version used when creating this model.
	CreatedAPIVersion string `json:"createdApiVersion"`
	// UpdatedAPIVersion is an api-version used when updating this model.
	UpdatedAPIVersion string `json:"updatedApiVersion,omitempty"`
	// AsyncProvisioningState is the provisioning state for async operation.
	AsyncProvisioningState ProvisioningState `json:"provisioningState,omitempty"`
}

// BaseResource represents common resource properties used for all resources.
type BaseResource struct {
	TrackedResource
	InternalMetadata

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData SystemData `json:"systemData,omitempty"`
}

// ResourceTypeName returns resource type name.
func (b *BaseResource) ResourceTypeName() string {
	return b.Type
}

// UpdateMetadata updates the default metadata with new request context and metadata in old resource.
func (b *BaseResource) UpdateMetadata(ctx *ARMRequestContext, oldResource *BaseResource) {
	if oldResource != nil {
		b.ID = oldResource.ID
		b.Name = oldResource.Name
		b.Type = oldResource.Type
		b.UpdatedAPIVersion = ctx.APIVersion
	} else {
		b.ID = ctx.ResourceID.String()
		b.Name = ctx.ResourceID.Name()
		b.Type = ctx.ResourceID.Type()
		b.CreatedAPIVersion = ctx.APIVersion
		b.UpdatedAPIVersion = ctx.APIVersion
	}

	b.Location = ctx.Location
	b.TenantID = ctx.HomeTenantID
}

// GetSystemdata gets systemdata.
func (b *BaseResource) GetSystemData() *SystemData {
	return &b.SystemData
}

// GetBaseResource gets internal base resource.
func (b *BaseResource) GetBaseResource() *BaseResource {
	return b
}

// ProvisioningState gets the provisioning state.
func (b *BaseResource) ProvisioningState() ProvisioningState {
	return b.InternalMetadata.AsyncProvisioningState
}

// SetProvisioningState sets the privisioning state of the resource.
func (b *BaseResource) SetProvisioningState(state ProvisioningState) {
	b.InternalMetadata.AsyncProvisioningState = state
}
