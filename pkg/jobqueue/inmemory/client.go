// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inmemory

import (
	"context"

	"github.com/project-radius/radius/pkg/jobqueue"
)

var _ jobqueue.Enqueuer = (*Client)(nil)
var _ jobqueue.Dequeuer = (*Client)(nil)

type Client struct {
	queue *inmemQueue
}

func NewClient() *Client {
	return &Client{
		queue: newInMemQueue(10),
	}
}

func (c *Client) Enqueue(ctx context.Context, msg *jobqueue.JobMessage, options ...jobqueue.EnqueueOptions) error {
	c.queue.Enqueue(msg)

	return nil
}

func (c *Client) Dequeue(ctx context.Context, options ...jobqueue.DequeueOptions) (<-chan jobqueue.JobMessageResponse, error) {
	out := make(chan jobqueue.JobMessageResponse, 1)

	go func() {
		for {
			msg := c.queue.Dequeue()

			out <- jobqueue.JobMessageResponse{
				Message: msg,
				Finish: func(err error) {
					if err != nil {
						c.queue.Complete(msg)
					}
				},
			}

			select {
			case <-ctx.Done():
				close(out)
				return
			}
		}
	}()

	return out, nil
}
