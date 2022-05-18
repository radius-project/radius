// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package jobqueue

import (
	"time"
)

// Metadata represents the metadata of queue message.
type Metadata struct {
	// ID represents the unique id of message.
	ID string
	// DequeueCount represents the number of dequeue.
	DequeueCount int
	// EnqueueTime represents the time when enqueuing the message
	EnqueueTime time.Time
	// ExpireTime represents the expiry of the message.
	ExpireTime time.Time
	// NextVisibleTime represents the next visible time after dequeuing the message.
	NextVisibleTime time.Time
}

type Message struct {
	Metadata

	ContentType string
	Data        interface{}

	finishFunc func(err error) error
	extendFunc func() error
}

func (m *Message) Finish(err error) error {
	if m.finishFunc != nil {
		return m.finishFunc(err)
	}
	return nil
}

func (m *Message) Extend(err error) error {
	if m.extendFunc != nil {
		return m.extendFunc()
	}
	return nil
}

func WithFinish(m *Message, finishFn func(err error) error, extendFn func() error) *Message {
	m.finishFunc = finishFn
	m.extendFunc = extendFn
	return m
}

func WithExtend(m *Message, extendFn func() error) *Message {
	m.extendFunc = extendFn
	return m
}
