// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"context"
	"sync"
	"time"

	"github.com/project-radius/radius/pkg/queue"
)

const dequeueInterval = 5 * time.Millisecond

var namedQueue = &sync.Map{}
var _ queue.Client = (*Client)(nil)

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
func (c *Client) Enqueue(ctx context.Context, msg *queue.Message, options ...queue.EnqueueOptions) error {
	c.queue.Enqueue(msg)
	return nil
}

// Dequeue dequeues message from the in-memory queue.
func (c *Client) Dequeue(ctx context.Context, options ...queue.DequeueOptions) (<-chan *queue.Message, error) {
	out := make(chan *queue.Message, 1)

	go func() {
		for {
			msg := c.queue.Dequeue()
			if msg != nil {
				msg.WithFinish(func(err error) error {
					return c.queue.Complete(msg)
				})
				msg.WithExtend(func() error {
					msg.NextVisibleAt = msg.NextVisibleAt.Add(messageLockDuration)
					return nil
				})
				out <- msg
			}

			select {
			case <-ctx.Done():
				close(out)
				return
			case <-time.After(dequeueInterval):
			}
		}
	}()

	return out, nil
}
