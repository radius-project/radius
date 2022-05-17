// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package jobqueue

import (
	"context"
)

// Enqueuer is an interface to enqueue Message to the job queue.
type Enqueuer interface {
	// Enqueue enqueues job message to the job queue.
	Enqueue(context.Context, *JobMessage, ...EnqueueOptions) error
}

// Dequeuer is an interface to dequeue job message from the job queue.
type Dequeuer interface {
	// Dequeue dequeues job message from the job queue.
	Dequeue(context.Context, ...DequeueOptions) (<-chan JobMessageResponse, error)
}
