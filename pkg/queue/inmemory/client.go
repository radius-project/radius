// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"context"
	"time"

	"github.com/project-radius/radius/pkg/queue"
)

var _ queue.Enqueuer = (*Client)(nil)
var _ queue.Dequeuer = (*Client)(nil)

// Client is the queue client used for dev and test purpose.
type Client struct {
	queue *inmemQueue
}

// NewClient creates the in-memory queue Client instance.
func NewClient() *Client {
	return &Client{
		queue: defaultQueue,
	}
}

// Enqueue enqueues message to the in-memory queue.
func (c *Client) Enqueue(ctx context.Context, msg *queue.Message, options ...queue.EnqueueOptions) error {
	c.queue.Enqueue(msg)
	return nil
}

// Dequeue dequeus message from in-memory queue.
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
			case <-time.After(5 * time.Millisecond):
			}
		}
	}()

	return out, nil
}
