// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1

import (
	"net/http"
	"strings"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/rp"
)

const (
	// DefaultRetryAfter is the default value in seconds for the Retry-After header.
	DefaultRetryAfter = "60"
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
func (o OperationMethod) HTTPMethod() string {
	m, ok := operationMethodToHTTPMethod[o]
	if !ok {
		// ARM RPC defines CRUD_L operations of one resource type and the custom action should be defined as POST method.
		// For example, if we want to support `listSecret` API for mongodatabase, this API must be defined as POST method.
		// POST /subscriptions/{subId}/resourcegroups/{rg}/applications.connectors/mongodatabases/{mongo}/listSecret
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
func (o OperationType) String() string {
	return strings.ToUpper(o.Type + Seperator + string(o.Method))
}

// ParseOperationType parses OperationType from string.
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
	ProvisioningStateFailed       ProvisioningState = "Failed"
	ProvisioningStateCanceled     ProvisioningState = "Canceled"
	ProvisioningStateUndefined    ProvisioningState = "Undefined"
)

// IsTerminal returns true if given Provisioning State is in a terminal state.
func (state ProvisioningState) IsTerminal() bool {
	return state == ProvisioningStateSucceeded || state == ProvisioningStateFailed || state == ProvisioningStateCanceled
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

	// ComputedValues map is any resource values that will be needed for more operations.
	// For example; database name to generate secrets for cosmos DB.
	ComputedValues map[string]interface{} `json:"computedValues,omitempty"`

	// Stores action to retrieve secret values. For Azure, connectionstring is accessed through cosmos listConnectionString operation, if secrets are not provided as input
	SecretValues map[string]rp.SecretValueReference `json:"secretValues,omitempty"`
}

type BasicResourceProperties struct {
	Status ResourceStatus `json:"status,omitempty"`
}

type ResourceStatus struct {
	OutputResources []outputresource.OutputResource `json:"outputResources,omitempty"`
}

// OutputResource contains some internal fields like resources/dependencies that shouldn't be inlcuded in the user response
func BuildExternalOutputResources(outputResources []outputresource.OutputResource) []map[string]interface{} {
	var externalOutputResources []map[string]interface{}
	for _, or := range outputResources {
		externalOutput := map[string]interface{}{
			"LocalID":  or.LocalID,
			"Provider": or.ResourceType.Provider,
			"Identity": or.Identity.Data,
		}
		externalOutputResources = append(externalOutputResources, externalOutput)
	}

	return externalOutputResources
}
