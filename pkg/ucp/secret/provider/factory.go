// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"os"

	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/secret/etcd"
	kubernetes_client "github.com/project-radius/radius/pkg/ucp/secret/kubernetes"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	radiusNamespace = "radius-system"
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
	return &etcd.Client{Storage: secretsStorageClient}, nil
}

func initKubernetesSecretsInterface(ctx context.Context, opt SecretProviderOptions) (secret.Client, error) {
	kubeconfig := os.Getenv("HOME") + "/.kube/config"
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	secretsClient := clientset.CoreV1().Secrets(radiusNamespace)
	return &kubernetes_client.Client{KubernetesSecretClient: secretsClient}, nil
}
