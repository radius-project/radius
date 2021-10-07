// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import "github.com/Azure/radius/pkg/azure/azresources"

const (
	Port         = 6379
	ResourceType = "redislabs.com.RedisComponent"
)

// RedisComponentProperties is the defintion of the config section
type RedisComponentProperties struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}

var RedisResourceType = azresources.KnownType{
	Types: []azresources.ResourceType{
		{
			Type: azresources.CacheRedis,
			Name: "*",
		},
	},
}
