// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretsprovider

import (
	"context"
	"os"

	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/secrets"
	"github.com/project-radius/radius/pkg/ucp/secrets/etcdsecrets"
	"github.com/project-radius/radius/pkg/ucp/secrets/kubernetessecrets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	radiusNamespace = "radius-system"
)

type secretsFactoryFunc func(context.Context, SecretsProviderOptions) (secrets.Interface, error)

var secretsClientFactory = map[SecretsProviderType]secretsFactoryFunc{
	TypeETCDSecrets:       initETCDSecretsInterface,
	TypeKubernetesSecrets: initKubernetesSecretsInterface,
}

func initETCDSecretsInterface(ctx context.Context, opt SecretsProviderOptions) (secrets.Interface, error) {
	secretsStorageClient, err := dataprovider.InitETCDClient(ctx, dataprovider.StorageProviderOptions{
		ETCD: opt.ETCD,
	}, "")
	if err != nil {
		return nil, err
	}
	return &etcdsecrets.Client{SecretsStorageClient: secretsStorageClient}, nil
}

func initKubernetesSecretsInterface(ctx context.Context, opt SecretsProviderOptions) (secrets.Interface, error) {
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
	return &kubernetessecrets.Client{SecretsClient: secretsClient}, nil
}
