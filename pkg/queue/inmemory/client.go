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

// Client is the in-memory queue client.
type Client struct {
	queue *inmemQueue
}

// NewClient creates the in-memory queue Client instance.
func NewClient() *Client {
	return &Client{
		queue: newInMemQueue(),
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

			msg.WithFinish(func(err error) error {
				if err != nil {
					c.queue.Complete(msg)
				}
				return nil
			}).WithExtend(func() error {
				msg.NextVisibleAt.Add(5 * time.Minute)
				return nil
			})

			out <- msg

			select {
			case <-ctx.Done():
				close(out)
				return
			}
		}
	}()

	return out, nil
}
