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
	return qinmem.NewNamedQueue(opt.InMemory.Name), nil
}
