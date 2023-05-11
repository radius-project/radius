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
	"context"
	"errors"

	"github.com/project-radius/radius/pkg/kubeutil"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/secret/etcd"
	kubernetes_client "github.com/project-radius/radius/pkg/ucp/secret/kubernetes"
	"github.com/project-radius/radius/pkg/ucp/store/etcdstore"
	"k8s.io/kubectl/pkg/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

type secretFactoryFunc func(context.Context, SecretProviderOptions) (secret.Client, error)

var secretClientFactory = map[SecretProviderType]secretFactoryFunc{
	TypeETCDSecret:       initETCDSecretClient,
	TypeKubernetesSecret: initKubernetesSecretClient,
}

func initETCDSecretClient(ctx context.Context, opts SecretProviderOptions) (secret.Client, error) {
	// etcd is a separate process run for development storage.
	// data provider already creates an etcd process which can be re-used instead of a new process for secret.
	client, err := dataprovider.InitETCDClient(ctx, dataprovider.StorageProviderOptions{
		ETCD: opts.ETCD,
	}, "")
	if err != nil {
		return nil, err
	}
	secretClient, ok := client.(*etcdstore.ETCDClient)
	if !ok {
		return nil, errors.New("no etcd Client detected")
	}
	return &etcd.Client{ETCDClient: secretClient.Client()}, nil
}

func initKubernetesSecretClient(ctx context.Context, opt SecretProviderOptions) (secret.Client, error) {
	s := scheme.Scheme
	cfg, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		// TODO: Allow to use custom context via configuration. - https://github.com/project-radius/radius/issues/5433
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
