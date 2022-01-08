// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import "github.com/project-radius/radius/pkg/azure/azresources"

const (
	Port         = 6379
	ResourceType = "redislabs.com.RedisCache"
)

var RedisResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.CacheRedis,
			Name: "*",
		},
	},
}
