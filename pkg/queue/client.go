// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package queue

import (
	"context"
)

// Enqueuer is an interface to enqueue Message to queue.
type Enqueuer interface {
	// Enqueue enqueues message to the job queue.
	Enqueue(context.Context, *Message, ...EnqueueOptions) error
}

// Dequeuer is an interface to dequeue job message from queue.
type Dequeuer interface {
	// Dequeue dequeues message from the queue.
	Dequeue(context.Context, ...DequeueOptions) (<-chan *Message, error)
}
