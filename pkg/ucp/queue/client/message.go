/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

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
