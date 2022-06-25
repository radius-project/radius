// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package client

import (
	"time"
)

const (
	JSONContentType = "application/json"
)

// Message represents message managed by queue.
type Message struct {
	Metadata

	ContentType string
	Data        []byte
}

// Metadata represents the metadata of queue message.
type Metadata struct {
	// ID represents the unique id of message.
	ID string
	// DequeueCount represents the number of dequeue.
	DequeueCount int
	// EnqueueAt represents the time when enqueuing the message
	EnqueueAt time.Time
	// ExpireAt represents the expiry of the message.
	ExpireAt time.Time
	// NextVisibleAt represents the next visible time after dequeuing the message.
	NextVisibleAt time.Time
}
