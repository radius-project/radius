// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	context "context"

	"github.com/project-radius/radius/pkg/queue"
	qinmem "github.com/project-radius/radius/pkg/queue/inmemory"
)

type factoryFunc func(context.Context, QueueProviderOptions) (queue.Client, error)

var clientFactory = map[QueueProviderType]factoryFunc{
	TypeInmemory: initInMemory,
}

func initInMemory(ctx context.Context, opt QueueProviderOptions) (queue.Client, error) {
	queueName := string(TypeInmemory)
	if opt.InMemory != nil && opt.InMemory.Name != "" {
		queueName = opt.InMemory.Name
	}
	return qinmem.NewNamedQueue(queueName), nil
}
