// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package datamodel

import (
	"errors"
	"strconv"

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

func (r *RedisCache) Transform(outputResources []outputresource.OutputResource, computedValues map[string]any, secretValues map[string]rp.SecretValueReference) error {
	r.Properties.Status.OutputResources = outputResources
	r.ComputedValues = computedValues
	r.SecretValues = secretValues
	if host, ok := computedValues[linkrp.Host].(string); ok {
		r.Properties.Host = host
	}
	if port, ok := computedValues[linkrp.Port]; ok {
		if port != nil {
			switch p := port.(type) {
			case float64:
				r.Properties.Port = int32(p)
			case int32:
				r.Properties.Port = p
			case string:
				converted, err := strconv.Atoi(p)
				if err != nil {
					return err
				}
				r.Properties.Port = int32(converted)
			default:
				return errors.New("unhandled type for the property port")
			}
		}
	}
	if username, ok := computedValues[linkrp.UsernameStringValue].(string); ok {
		r.Properties.Username = username
	}

	return nil
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

// ComputedValues returns the computed values on the link.
func (r *RedisCache) GetComputedValues() map[string]any {
	return r.LinkMetadata.ComputedValues
}

// SecretValues returns the secret values for the link.
func (r *RedisCache) GetSecretValues() map[string]rp.SecretValueReference {
	return r.LinkMetadata.SecretValues
}

// RecipeData returns the recipe data for the link.
func (r *RedisCache) GetRecipeData() RecipeData {
	return r.LinkMetadata.RecipeData
}

func (redis *RedisCache) ResourceTypeName() string {
	return "Applications.Link/redisCaches"
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
	return "Applications.Link/redisCaches"
}
