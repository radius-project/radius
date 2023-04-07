// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
	cfg, err := kubeutil.NewClusterConfig("")
	if err != nil {
		return nil, err
	}
	client, err := controller_runtime.New(cfg, controller_runtime.Options{Scheme: s})
	if err != nil {
		return nil, err
	}
	return &kubernetes_client.Client{K8sClient: client}, nil
}
