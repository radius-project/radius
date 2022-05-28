// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import (
	"time"

	"github.com/google/uuid"
)

var (
	// DefaultAsyncOperationTimeout is the default timeout duration of async operation.
	DefaultAsyncOperationTimeout = 1 * time.Hour
)

// RequestMessage is a message used for async request queue message broker.
type RequestMessage struct {
	// OperationID represents the unique id of the async operation.
	OperationID uuid.UUID `json:"asyncOperationID"`
	// OperationName represents the name of operation.
	OperationName string `json:"operationName"`
	// ResourceID represents the id of the resource which requires async operation.
	ResourceID string `json:"resourceID"`

	// CorrelationID represents the correlation ID of async operation.
	CorrelationID string `json:"correlationID,omitempty"`
	// TraceparentID represents W3C trace parent ID of async operation.
	TraceparentID string `json:"traceparent,omitempty"`
	// AcceptLanguage represents the locale of operation request.
	AcceptLanguage string `json:"language,omitempty"`

	// HomeTenantID represents the home tenant id of caller.
	HomeTenantID string `json:"homeTenantID,omitempty"`
	// ClientObjectID represents the client object id of caller.
	ClientObjectID string `json:"clientObjectID,omitempty"`

	// OperationTimeout represents the timeout duration of async operation.
	OperationTimeout time.Duration `json:"asyncOperationTimeout"`
}
