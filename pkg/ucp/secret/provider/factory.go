// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"errors"

	"github.com/project-radius/radius/pkg/rp/k8sauth"
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
	TypeETCDSecrets:       initETCDSecretsInterface,
	TypeKubernetesSecrets: initKubernetesSecretsInterface,
}

func initETCDSecretsInterface(ctx context.Context, opt SecretProviderOptions) (secret.Client, error) {
	secretsStorageClient, err := dataprovider.InitETCDClient(ctx, dataprovider.StorageProviderOptions{
		ETCD: opt.ETCD,
	}, "")
	if err != nil {
		return nil, err
	}
	secretClient, ok := secretsStorageClient.(*etcdstore.ETCDClient)
	if !ok {
		return nil, errors.New("No etcd Client detected")
	}
	return &etcd.Client{ETCDClient: secretClient.Client}, nil
}

func initKubernetesSecretsInterface(ctx context.Context, opt SecretProviderOptions) (secret.Client, error) {
	s := scheme.Scheme
	config, err := k8sauth.GetConfig()
	if err != nil {
		return nil, err
	}
	secretsClient, err := controller_runtime.New(config, controller_runtime.Options{Scheme: s})
	if err != nil {
		return nil, err
	}
	return &kubernetes_client.Client{K8sClient: secretsClient}, nil
}
