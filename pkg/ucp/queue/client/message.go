// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package client

import (
	"encoding/json"
	"time"
)

const (
	// JSONContentType represents the json content type of queue message.
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

// NewMessage creates Message.
func NewMessage(data any) *Message {
	msg := &Message{
		// Support only JSONContentType.
		ContentType: JSONContentType,
	}

	switch d := data.(type) {
	case []byte:
		msg.Data = d
	case string:
		msg.Data = []byte(d)
	default:
		var err error
		msg.ContentType = JSONContentType
		msg.Data, err = json.Marshal(data)
		if err != nil {
			return nil
		}
	}

	return msg
}
