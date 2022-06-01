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

type OperationType struct {
	TypeName string
	Method   string
}

func (o OperationType) String() string {
	return strings.ToUpper(o.TypeName + Seperator + o.Method)
}

func ParseOperationTypeFromString(s string) (OperationType, bool) {
	p := strings.Split(s, Seperator)
	if len(p) >= 2 {
		return OperationType{TypeName: strings.ToUpper(p[0]), Method: strings.ToUpper(p[1])}, true
	}
	return OperationType{}, false
}
