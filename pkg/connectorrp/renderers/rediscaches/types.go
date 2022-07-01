// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	ResourceType = "Applications.Connector/redisCaches"
)

var RedisResourceType = resources.KnownType{
	Types: []resources.TypeSegment{
		{
			Type: azresources.CacheRedis,
			Name: "*",
		},
	},
}
