// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"errors"
	"sync"

	queue "github.com/project-radius/radius/pkg/ucp/queue/client"
)

var (
	ErrUnsupportedStorageProvider = errors.New("unsupported queue provider")
)

// QueueProvider is the provider to create and manage queue client.
type QueueProvider struct {
	queueClient queue.Client
	once        sync.Once
	options     QueueProviderOptions
}

// New creates new QueueProvider instance.
func New(opts QueueProviderOptions) *QueueProvider {
	return &QueueProvider{
		queueClient: nil,
		options:     opts,
	}
}

// GetClient creates or gets queue client.
func (p *QueueProvider) GetClient(ctx context.Context) (queue.Client, error) {
	if p.queueClient != nil {
		return p.queueClient, nil
	}

	err := ErrUnsupportedStorageProvider
	p.once.Do(func() {
		if fn, ok := clientFactory[p.options.Provider]; ok {
			p.queueClient, err = fn(ctx, p.options)
		}
	})

	return p.queueClient, err
}
