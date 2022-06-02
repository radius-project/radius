// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import "strings"

const (
	// DefaultRetryAfter is the default value in seconds for the Retry-After header.
	DefaultRetryAfter = "60"
)

const (
	// Predefined Operation methods.
	OperationList                 = "LIST"
	OperationPut                  = "PUT"
	OperationPatch                = "PATCH"
	OperationGet                  = "GET"
	OperationDelete               = "DELETE"
	OperationGetOperations        = "GETOPERATIONS"
	OperationGetOperationStatuses = "GETOPERATIONSTATUSES"
	OperationGetOperationResult   = "GETOPERATIONRESULT"
	OperationPutSubscriptions     = "PUTSUBSCRIPTIONS"

	Seperator = "|"
)

// OperationType represents the operation type which includes resource type name and its method.
// OperationType is used as a route name in the frontend API server router. Each valid ARM RPC call should have
// its own operation type name. For Asynchronous API, the frontend API server queues the async operation
// request with this operation type. AsyncRequestProcessWorker parses the operation type from the message
// and run the corresponding async operation controller.
type OperationType struct {
	Type   string
	Method string
}

// String returns the operation type string.
func (o OperationType) String() string {
	return strings.ToUpper(o.Type + Seperator + o.Method)
}

// ParseOperationType parses OperationType from string.
func ParseOperationType(s string) (OperationType, bool) {
	p := strings.Split(s, Seperator)
	if len(p) == 2 {
		return OperationType{Type: strings.ToUpper(p[0]), Method: strings.ToUpper(p[1])}, true
	}
	return OperationType{}, false
}
