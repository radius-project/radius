// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package jobqueue

import "time"

type JobMessageContext struct {
	DequeueCount    int
	EnqueueTime     time.Time
	ExpireTime      time.Time
	NextVisibleTime time.Time
}

type JobMessage struct {
	JobMessageContext

	// ID represents the unique id of JobMessage
	ID string `json:"id"`
	// ResourceID represents the id of the resource which requires async operation.
	ResourceID string `json:"resourceID"`
	// AsyncOperationID represents the unique id of the async operation.
	AsyncOperationID string `json:"asyncOperationID"`
	// OperationName represents the name of operation.
	OperationName string `json:"operationName"`

	// JobStartTime represents the start time of async operation.
	JobStartTime time.Time `json:"jobStartTime"`

	// CorrelationID represents the correlation ID of async operation.
	CorrelationID string `json:"correlationID,omitempty"`
	// TraceparentID represents W3C trace parent ID of async operation.
	TraceparentID string `json:"traceparent,omitempty"`
	// AcceptLanguage represents the locale of operation request.
	AcceptLanguage string `json:"language,omitempty"`
}

func NewJobMessage(id string) *JobMessage {
	return &JobMessage{
		JobMessageContext: JobMessageContext{
			DequeueCount:    0,
			EnqueueTime:     time.Now().UTC(),
			ExpireTime:      time.Now().UTC().Add(time.Hour * 48),
			NextVisibleTime: time.Now().UTC(),
		},
	}
}

type JobMessageResponse struct {
	Message *JobMessage
	Finish  func(err error)
}
