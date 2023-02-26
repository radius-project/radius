// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package client

import (
	"context"
	"errors"
	"time"

	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var (
	// ErrDequeuedMessage represents the error when message has already been dequeued.
	ErrDequeuedMessage = errors.New("message was dequeued by the other client")

	// ErrMessageNotFound represents the error when queue is empty or all messages are leased by clients.
	ErrMessageNotFound = errors.New("queue is empty or messages are leased")

	// ErrInvalidMessage represents the error when the message has already been requeued.
	ErrInvalidMessage = errors.New("this message has been requeued or deleted")

	// ErrUnsupportedContentType represents the error when the content type is unsupported.
	ErrUnsupportedContentType = errors.New("this message content type is unsupported")

	// ErrEmptyMessage represents nil or empty Message.
	ErrEmptyMessage = errors.New("message must not be nil or message is empty")

	dequeueInterval = time.Duration(5) * time.Millisecond
)

//go:generate mockgen -destination=./mock_client.go -package=client -self_package github.com/project-radius/radius/pkg/ucp/queue/client github.com/project-radius/radius/pkg/ucp/queue/client Client

// Client is an interface to implement queue operations.
type Client interface {
	// Enqueue enqueues message to queue.
	Enqueue(ctx context.Context, msg *Message, opts ...EnqueueOptions) error

	// Dequeue dequeues message from queue.
	Dequeue(ctx context.Context, opts ...DequeueOptions) (*Message, error)

	// FinishMessage finishes or deletes the message in the queue.
	FinishMessage(ctx context.Context, msg *Message) error

	// ExtendMessage extends the message lock.
	ExtendMessage(ctx context.Context, msg *Message) error
}

// StartDequeuer starts a dequeuer to consume the message from the queue and return the output channel.
func StartDequeuer(ctx context.Context, cli Client, opts ...DequeueOptions) (<-chan *Message, error) {
	log := ucplog.FromContext(ctx)
	out := make(chan *Message, 1)

	go func() {
		for {
			msg, err := cli.Dequeue(ctx, opts...)
			if err == nil {
				out <- msg
			} else if !errors.Is(err, ErrMessageNotFound) {
				log.Error(err, "fails to dequeue the message")
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
