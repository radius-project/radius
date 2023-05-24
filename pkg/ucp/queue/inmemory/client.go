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

package inmemory

import (
	"context"
	"sync"

	"github.com/project-radius/radius/pkg/ucp/queue/client"
)

var namedQueue = &sync.Map{}
var _ client.Client = (*Client)(nil)

// Client is the queue client used for dev and test purpose.
type Client struct {
	queue *InmemQueue
}

// New creates the in-memory queue Client instance. Client will use the default global queue if queue is nil.
func New(queue *InmemQueue) *Client {
	if queue == nil {
		queue = defaultQueue
	}

	return &Client{
		queue: queue,
	}
}

// New creates the named in-memory queue Client instance.
func NewNamedQueue(name string) *Client {
	inmemq, _ := namedQueue.LoadOrStore(name, NewInMemQueue(messageLockDuration))
	return &Client{
		queue: inmemq.(*InmemQueue),
	}
}

// Enqueue enqueues message to the in-memory queue.
func (c *Client) Enqueue(ctx context.Context, msg *client.Message, options ...client.EnqueueOptions) error {
	if msg == nil || msg.Data == nil || len(msg.Data) == 0 {
		return client.ErrEmptyMessage
	}
	c.queue.Enqueue(msg)
	return nil
}

// Dequeue dequeues message from the in-memory queue.
func (c *Client) Dequeue(ctx context.Context, opts ...client.DequeueOptions) (*client.Message, error) {
	msg := c.queue.Dequeue()
	if msg == nil {
		return nil, client.ErrMessageNotFound
	}
	return msg, nil
}

// FinishMessage finishes or deletes the message in the queue.
func (c *Client) FinishMessage(ctx context.Context, msg *client.Message) error {
	if msg == nil {
		return client.ErrEmptyMessage
	}

	return c.queue.Complete(msg)
}

// ExtendMessage extends the message lock.
func (c *Client) ExtendMessage(ctx context.Context, msg *client.Message) error {
	if msg == nil {
		return client.ErrEmptyMessage
	}

	err := c.queue.Extend(msg)
	if err != nil {
		return err
	}
	return err
}
