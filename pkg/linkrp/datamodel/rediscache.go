// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
)

// RedisCache represents RedisCache link resource.
type RedisCache struct {
	v1.TrackedResource

	// SystemData is the systemdata which includes creation/modified dates.
	SystemData v1.SystemData `json:"systemData,omitempty"`
	// Properties is the properties of the resource.
	Properties RedisCacheProperties `json:"properties"`

	// InternalMetadata is the internal metadata which is used for conversion.
	v1.InternalMetadata

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (redis RedisCache) ResourceTypeName() string {
	return "Applications.Link/redisCaches"
}

func (redisSecrets RedisCacheSecrets) IsEmpty() bool {
	return redisSecrets == RedisCacheSecrets{}
}

type RedisValuesProperties struct {
	Host     string `json:"host,omitempty"`
	Port     int32  `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
}

type RedisResourceProperties struct {
	Resource string `json:"resource,omitempty"`
}

type RedisRecipeProperties struct {
	Recipe LinkRecipe `json:"recipe,omitempty"`
}
type RedisCacheProperties struct {
	rp.BasicResourceProperties
	RedisValuesProperties
	RedisResourceProperties
	RedisRecipeProperties
	ProvisioningState v1.ProvisioningState `json:"provisioningState,omitempty"`
	Secrets           RedisCacheSecrets    `json:"secrets,omitempty"`
	Mode              LinkMode             `json:"mode"`
}

// Secrets values consisting of secrets provided for the resource
type RedisCacheSecrets struct {
	ConnectionString string `json:"connectionString"`
	Password         string `json:"password"`
}

func (redis RedisCacheSecrets) ResourceTypeName() string {
	return "Applications.Link/redisCaches"
}
