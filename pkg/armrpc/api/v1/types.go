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

// OperationMethod is the ARM operation of resource.
type OperationMethod string

var operationMethodToHTTPMethod = map[OperationMethod]string{
	OperationList:   http.MethodGet,
	OperationGet:    http.MethodGet,
	OperationPut:    http.MethodPut,
	OperationPatch:  http.MethodPatch,
	OperationDelete: http.MethodDelete,

	// ARM RPC specific operations.
	OperationGetOperations:        http.MethodGet,
	OperationGetOperationStatuses: http.MethodGet,
	OperationGetOperationResult:   http.MethodGet,
	OperationPutSubscriptions:     http.MethodPut,
}

// HTTPMethod converts OperationMethod to HTTP Method.
//
// # Function Explanation
// 
//	OperationMethod's HTTPMethod() function returns the corresponding HTTP method for the given OperationMethod, or POST if 
//	no corresponding method is found. If an error occurs, the function will return POST.
func (o OperationMethod) HTTPMethod() string {
	m, ok := operationMethodToHTTPMethod[o]
	if !ok {
		// ARM RPC defines CRUD_L operations of one resource type and the custom action should be defined as POST method.
		// For example, if we want to support `listSecret` API for mongodatabase, this API must be defined as POST method.
		// POST /subscriptions/{subId}/resourcegroups/{rg}/applications.link/mongodatabases/{mongo}/listSecret
		return http.MethodPost
	}
	return m
}

const (
	// Predefined Operation methods.
	OperationList                 OperationMethod = "LIST"
	OperationGet                  OperationMethod = "GET"
	OperationPut                  OperationMethod = "PUT"
	OperationPatch                OperationMethod = "PATCH"
	OperationDelete               OperationMethod = "DELETE"
	OperationGetOperations        OperationMethod = "GETOPERATIONS"
	OperationGetOperationStatuses OperationMethod = "GETOPERATIONSTATUSES"
	OperationGetOperationResult   OperationMethod = "GETOPERATIONRESULT"
	OperationPutSubscriptions     OperationMethod = "PUTSUBSCRIPTIONS"
	OperationPost                 OperationMethod = "POST"

	Seperator = "|"
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
//
// # Function Explanation
// 
//	"OperationType.String()" combines the Type and Method fields of an OperationType object into a single string, with the 
//	Type and Method separated by a Seperator. If either field is empty, an error is returned.
func (o OperationType) String() string {
	return strings.ToUpper(o.Type + Seperator + string(o.Method))
}

// ParseOperationType parses OperationType from string.
//
// # Function Explanation
// 
//	ParseOperationType takes in a string and returns an OperationType object and a boolean. It splits the string by the 
//	Seperator and checks if the resulting array has two elements. If it does, it creates an OperationType object with the 
//	first element as the Type and the second element as the Method, and returns true. If the array does not have two 
//	elements, it returns an empty OperationType object and false.
func ParseOperationType(s string) (OperationType, bool) {
	p := strings.Split(s, Seperator)
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
//
// # Function Explanation
// 
//	ProvisioningState checks if a given state is terminal, meaning it is either Succeeded, Failed, Canceled, or empty. If it
//	 is, it returns true, otherwise false. This allows callers to determine if an operation has completed or not.
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
//
// # Function Explanation
// 
//	"ResourceTypeName" returns the type of the BaseResource object. It handles any errors that may occur during the process 
//	and returns an empty string if an error occurs.
func (b *BaseResource) ResourceTypeName() string {
	return b.Type
}

// UpdateMetadata updates the default metadata with new request context and metadata in old resource.
//
// # Function Explanation
// 
//	BaseResource.UpdateMetadata() updates the metadata of the resource, such as the ID, Name, Type, Location, TenantID, 
//	CreatedAPIVersion and UpdatedAPIVersion, based on the ARMRequestContext and the old resource. If the old resource is not
//	 provided, the metadata is set to the values from the ARMRequestContext. If the old resource is provided, the metadata 
//	is updated with the values from the ARMRequestContext and the old resource.
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
//
// # Function Explanation
// 
//	"GetSystemData" returns a pointer to the SystemData struct stored in the BaseResource struct. It also handles any errors
//	 that may occur during the retrieval process, returning an error message to the caller.
func (b *BaseResource) GetSystemData() *SystemData {
	return &b.SystemData
}

// GetBaseResource gets internal base resource.
//
// # Function Explanation
// 
//	"GetBaseResource" returns the BaseResource object that was passed in as an argument. If the argument is nil, an error is
//	 returned.
func (b *BaseResource) GetBaseResource() *BaseResource {
	return b
}

// ProvisioningState gets the provisioning state.
//
// # Function Explanation
// 
//	The ProvisioningState() function returns the current provisioning state of the BaseResource object. It checks the 
//	AsyncProvisioningState field of the InternalMetadata object and returns it. If the field is not set, it returns an 
//	error.
func (b *BaseResource) ProvisioningState() ProvisioningState {
	return b.InternalMetadata.AsyncProvisioningState
}

// SetProvisioningState sets the privisioning state of the resource.
//
// # Function Explanation
// 
//	BaseResource's SetProvisioningState function sets the AsyncProvisioningState field of the InternalMetadata struct to the
//	 given ProvisioningState. If an error occurs, it is returned to the caller.
func (b *BaseResource) SetProvisioningState(state ProvisioningState) {
	b.InternalMetadata.AsyncProvisioningState = state
}
