// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
)

// RedisCache represents RedisCache link resource.
type RedisCache struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RedisCacheProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	LinkMetadata
}

func (redis RedisCache) ResourceTypeName() string {
	return "Applications.Link/redisCaches"
}

func (redisSecrets RedisCacheSecrets) IsEmpty() bool {
	return redisSecrets == RedisCacheSecrets{}
}

func (redis RedisCache) ApplyDeploymentOutput(deploymentOutput rp.DeploymentOutput) {
	// TODO: fill this in when I understand how it's used.
}

func (redis RedisCache) OutputResources() []outputresource.OutputResource {
	return redis.Properties.Status.OutputResources
}

func (redis RedisCache) ResourceMetadata() *rp.BasicResourceProperties {
	return &redis.Properties.BasicResourceProperties
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
	Secrets RedisCacheSecrets `json:"secrets,omitempty"`
	Mode    LinkMode          `json:"mode"`
}

// Secrets values consisting of secrets provided for the resource
type RedisCacheSecrets struct {
	ConnectionString string `json:"connectionString"`
	Password         string `json:"password"`
	URL              string `json:"url"`
}

func (redis RedisCacheSecrets) ResourceTypeName() string {
	return "Applications.Link/redisCaches"
}
