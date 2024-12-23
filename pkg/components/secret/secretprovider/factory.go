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

package secretprovider

import (
	"context"

	"github.com/radius-project/radius/pkg/components/secret"
	"github.com/radius-project/radius/pkg/components/secret/inmemory"
	kubernetes_client "github.com/radius-project/radius/pkg/components/secret/kubernetes"
	"github.com/radius-project/radius/pkg/kubeutil"
	"k8s.io/kubectl/pkg/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

type secretFactoryFunc func(context.Context, SecretProviderOptions) (secret.Client, error)

var secretClientFactory = map[SecretProviderType]secretFactoryFunc{
	TypeKubernetesSecret: initKubernetesSecretClient,
	TypeInMemorySecret:   initInMemorySecretClient,
}

func initKubernetesSecretClient(ctx context.Context, opt SecretProviderOptions) (secret.Client, error) {
	s := scheme.Scheme
	cfg, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		// TODO: Allow to use custom context via configuration. - https://github.com/radius-project/radius/issues/5433
		ContextName: "",
		QPS:         kubeutil.DefaultServerQPS,
		Burst:       kubeutil.DefaultServerBurst,
	})
	if err != nil {
		return nil, err
	}
	client, err := controller_runtime.New(cfg, controller_runtime.Options{Scheme: s})
	if err != nil {
		return nil, err
	}
	return &kubernetes_client.Client{K8sClient: client}, nil
}

func initInMemorySecretClient(ctx context.Context, opt SecretProviderOptions) (secret.Client, error) {
	return &inmemory.Client{}, nil
}
