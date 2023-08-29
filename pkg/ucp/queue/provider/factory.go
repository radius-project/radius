/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	context "context"
	"errors"
	"fmt"

	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/ucp/queue/apiserver"
	queue "github.com/radius-project/radius/pkg/ucp/queue/client"
	qinmem "github.com/radius-project/radius/pkg/ucp/queue/inmemory"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/ucp/store/apiserverstore/api/ucp.dev/v1alpha1"
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

	cfg, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: opt.APIServer.Context,
		QPS:         kubeutil.DefaultServerQPS,
		Burst:       kubeutil.DefaultServerBurst,
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
		WarningHandler: runtimeclient.WarningHandlerOptions{
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
