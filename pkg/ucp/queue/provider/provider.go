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
	"github.com/project-radius/radius/pkg/ucp/util"
)

var (
	ErrUnsupportedStorageProvider = errors.New("unsupported queue provider")
)

// QueueProvider is the provider to create and manage queue client.
type QueueProvider struct {
	name    string
	options QueueProviderOptions

	queueClient queue.Client
	once        sync.Once
}

// New creates new QueueProvider instance.
func New(name string, opts QueueProviderOptions) *QueueProvider {
	return &QueueProvider{
		name:        util.NormalizeStringToLower(name),
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
			p.queueClient, err = fn(ctx, p.name, p.options)
		}
	})

	return p.queueClient, err
}
