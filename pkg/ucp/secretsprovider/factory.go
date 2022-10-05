// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretsprovider

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/secrets"
	"github.com/project-radius/radius/pkg/ucp/secrets/etcdsecrets"
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
	return nil, nil
}
