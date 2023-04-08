// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	context "context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/kubeutil"
	"github.com/project-radius/radius/pkg/ucp/queue/apiserver"
	queue "github.com/project-radius/radius/pkg/ucp/queue/client"
	qinmem "github.com/project-radius/radius/pkg/ucp/queue/inmemory"
	ucpv1alpha1 "github.com/project-radius/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type factoryFunc func(context.Context, string, QueueProviderOptions) (queue.Client, error)

var clientFactory = map[QueueProviderType]factoryFunc{
	TypeInmemory:  initInMemory,
	TypeAPIServer: initAPIServer,
}

func initInMemory(ctx context.Context, name string, opt QueueProviderOptions) (queue.Client, error) {
	return qinmem.NewNamedQueue(name), nil
}

func initAPIServer(ctx context.Context, name string, opt QueueProviderOptions) (queue.Client, error) {
	if opt.APIServer.Namespace == "" {
		return nil, errors.New("failed to initialize APIServer client: namespace is required")
	}

	cfg, err := kubeutil.NewClusterConfig(&kubeutil.ConfigOptions{
		ContextName: opt.APIServer.Context,
		QPS:         kubeutil.ServerQPS,
		Burst:       kubeutil.ServerBurst,
	})
	if err != nil {
		return nil, err
	}

	// We only need to interact with UCP's store types.
	scheme := runtime.NewScheme()

	// Safe to ignore, this will only fail for duplicates, which there clearly won't be.
	_ = ucpv1alpha1.AddToScheme(scheme)

	options := runtimeclient.Options{
		Scheme: scheme,

		// The client will log info the console that we don't really care about.
		Opts: runtimeclient.WarningHandlerOptions{
			SuppressWarnings: true,
		},
	}

	rc, err := runtimeclient.New(cfg, options)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize APIServer client: %w", err)
	}

	return apiserver.New(rc, apiserver.Options{
		Name:      name,
		Namespace: opt.APIServer.Namespace,
	})
}
