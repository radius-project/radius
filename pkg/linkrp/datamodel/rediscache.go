// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
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

// ApplyDeploymentOutput applies the properties changes based on the deployment output.
func (r *RedisCache) ApplyDeploymentOutput(do rp.DeploymentOutput) {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
}

// OutputResources returns the output resources array.
func (r *RedisCache) OutputResources() []outputresource.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *RedisCache) ResourceMetadata() *rp.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (redis *RedisCache) ResourceTypeName() string {
	return linkrp.RedisCachesResourceType
}

func (redisSecrets *RedisCacheSecrets) IsEmpty() bool {
	return redisSecrets == nil || *redisSecrets == RedisCacheSecrets{}
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
}

func (redis RedisCacheSecrets) ResourceTypeName() string {
	return linkrp.RedisCachesResourceType
}
