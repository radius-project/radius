// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package portforward

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
)

var _ Interface = (*Impl)(nil)

type Impl struct {
}

func (i *Impl) Run(ctx context.Context, options Options) error {
	if options.StatusChan != nil {
		defer close(options.StatusChan)
	}

	// We allow initialization of other context, or the client + config. This is the
	// most flexible for tests.
	if options.Client == nil && options.RESTConfig == nil {
		client, restConfig, err := kubernetes.NewClientset(options.KubeContext)
		if err != nil {
			return err
		}

		options.Client = client
		options.RESTConfig = restConfig
	}

	// The overall algorithm we're going to follow works like this:
	//
	// Up Front:
	// - Find the deployments that are relevant for the application
	// - Exclude any replicasets that are "old" - this command is frequently
	//   used right after a Radius deployment, so we want to ignore pods from the old
	//   replica set that are outdated.
	//
	// Then:
	// - Watch deployments in the application
	//   - For each deployment watch one pod and try to forward to it
	//     - If the pod shuts down then pick the newest other replice and forward to that
	watcher := NewApplicationWatcher(options)
	return watcher.Run(ctx)
}
