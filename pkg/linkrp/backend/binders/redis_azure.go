// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package binders

import (
	"context"

	azredis "github.com/Azure/azure-sdk-for-go/profiles/latest/redis/mgmt/redis"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

var _ Binder[*datamodel.RedisCache] = (*RedisAzureBinder)(nil)

type RedisAzureBinder struct {
}

// Bind implements Binder
func (b *RedisAzureBinder) Bind(ctx context.Context, id string, fetch FetchFunc, destination *datamodel.RedisCache, secrets map[string]rp.SecretValueReference) error {
	azureRedis := azredis.ResourceType{}
	err := fetch(ctx, &azureRedis, id, "2021-06-01")
	if err != nil {
		return err
	}

	destination.Properties.Host = *azureRedis.HostName
	destination.Properties.Port = *azureRedis.SslPort
	destination.Properties.Username = "" // Blank for Azure. YES THE USERNAME IS BLANK.

	if *azureRedis.EnableNonSslPort {
		destination.Properties.Port = *azureRedis.Port
	}

	secrets[renderers.PasswordStringHolder] = rp.SecretValueReference{
		LocalID:       outputresource.LocalIDAzureRedis,
		Action:        "listKeys",
		ValueSelector: "/primaryKey",
	}

	secrets[renderers.ConnectionStringValue] = rp.SecretValueReference{
		LocalID:       outputresource.LocalIDAzureRedis,
		Action:        "listKeys",
		ValueSelector: "/primaryKey",
		Transformer: resourcemodel.ResourceType{
			Provider: resourcemodel.ProviderAzure,
			Type:     resourcekinds.AzureRedis,
		},
	}

	secrets["url"] = rp.SecretValueReference{
		LocalID:       outputresource.LocalIDAzureRedis,
		Action:        "listKeys",
		ValueSelector: "/primaryKey",
		Transformer: resourcemodel.ResourceType{
			Provider: resourcemodel.ProviderAzure,
			Type:     "azure.redis.url",
		},
	}

	return nil
}
