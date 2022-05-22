// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package queue

import (
	"time"
)

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

// Message represents message managed by enqueuer and dequeuer.
type Message struct {
	Metadata

	Data interface{}

	finishFunc func(err error) error
	extendFunc func() error
}

// NewMessage creates Message object.
func NewMessage(data interface{}) *Message {
	return &Message{
		Data: data,
		finishFunc: func(err error) error {
			return nil
		},
		extendFunc: func() error {
			return nil
		},
	}
}

// WithFinish sets message finish function.
func (m *Message) WithFinish(finishFn func(err error) error) *Message {
	m.finishFunc = finishFn
	return m
}

// Finish completes Message.
func (m *Message) Finish(err error) error {
	if m.finishFunc != nil {
		return m.finishFunc(err)
	}
	return nil
}

// WithExtend sets message lock extension function.
func (m *Message) WithExtend(extendFn func() error) *Message {
	m.extendFunc = extendFn
	return m
}

// Extend extends the message lock.
func (m *Message) Extend() error {
	if m.extendFunc != nil {
		return m.extendFunc()
	}
	return nil
}
