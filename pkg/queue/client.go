// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package client

import (
	"context"
)

//go:generate mockgen -destination=./mock_client.go -package=queue -self_package github.com/project-radius/radius/pkg/queue github.com/project-radius/radius/pkg/queue Client

// Client is an interface to implement queue operations.
type Client interface {
	// Enqueue enqueues message to the job queue.
	Enqueue(context.Context, *Message, ...EnqueueOptions) error
	// Dequeue dequeues message from the queue.
	Dequeue(context.Context, ...DequeueOptions) (<-chan *Message, error)
}
