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

package controller

import (
	"time"

	"github.com/google/uuid"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var (
	// DefaultAsyncOperationTimeout is the default timeout duration of async operation.
	DefaultAsyncOperationTimeout = time.Duration(120) * time.Second
)

// Request is a message used for async request queue message broker.
type Request struct {
	// APIVersion represents the api-version of operation request.
	APIVersion string `json:"apiVersion"`

	// OperationID represents the unique id of the async operation.
	OperationID uuid.UUID `json:"asyncOperationID"`
	// OperationType represents the type of operation.
	OperationType string `json:"operationType"`
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
	OperationTimeout *time.Duration `json:"asyncOperationTimeout"`
}

// Timeout gets the async operation timeout duration.
func (r *Request) Timeout() time.Duration {
	if r.OperationTimeout == nil {
		return DefaultAsyncOperationTimeout
	}
	return *r.OperationTimeout
}

// ARMRequestContext creates the ARM request context from async operation request.
func (r *Request) ARMRequestContext() (*v1.ARMRequestContext, error) {
	rID, err := resources.Parse(r.ResourceID)
	if err != nil {
		return nil, err
	}

	opType, ok := v1.ParseOperationType(r.OperationType)
	if !ok {
		return nil, v1.ErrInvalidOperationType
	}

	rpcCtx := &v1.ARMRequestContext{
		ResourceID:    rID,
		CorrelationID: r.CorrelationID,
		OperationID:   r.OperationID,
		OperationType: opType,
		Traceparent:   r.TraceparentID,

		HomeTenantID:   r.HomeTenantID,
		ClientObjectID: r.ClientObjectID,

		APIVersion:     r.APIVersion,
		AcceptLanguage: r.AcceptLanguage,
	}

	return rpcCtx, nil
}
