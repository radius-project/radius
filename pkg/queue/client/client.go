// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package client

import (
	"context"
	"errors"
)

var (
	// ErrDeqeueudMessage represents the error when message has already been dequeued.
	ErrDeqeueudMessage = errors.New("message was dequeued by the other client")

	// ErrMessageNotFound represents the error when queue is empty or all messages are leased by clients.
	ErrMessageNotFound = errors.New("queue is empty or messages are leased")

	// ErrRequeuedMessage represents the error when the message has already been requeued.
	ErrRequeuedMessage = errors.New("this message has been requeued")
)

//go:generate mockgen -destination=./mock_client.go -package=client -self_package github.com/project-radius/radius/pkg/queue/client github.com/project-radius/radius/pkg/queue/client Client

// Client is an interface to implement queue operations.
type Client interface {
	// Enqueue enqueues message to queue.
	Enqueue(ctx context.Context, msg *Message, opts ...EnqueueOptions) error

	// Dequeue dequeues message from queue.
	Dequeue(ctx context.Context, opts ...DequeueOptions) (*Message, error)

	// StartDequeuer starts a dequeuer to consume the message from the queue and return the consumer channel.
	StartDequeuer(ctx context.Context, opts ...DequeueOptions) (<-chan *Message, error)

	// FinishMessage finishes or deletes the message in the queue.
	FinishMessage(ctx context.Context, msg *Message) error

	// ExtendMessage extends the message lock.
	ExtendMessage(ctx context.Context, msg *Message) error
}
