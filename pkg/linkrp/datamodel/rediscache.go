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
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
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
func (r *RedisCache) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	r.Properties.Status.OutputResources = do.DeployedOutputResources
	r.ComputedValues = do.ComputedValues
	r.SecretValues = do.SecretValues
	if host, ok := do.ComputedValues[renderers.Host].(string); ok {
		r.Properties.Host = host
	}
	if port, ok := do.ComputedValues[renderers.Port]; ok {
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
	if username, ok := do.ComputedValues[renderers.UsernameStringValue].(string); ok {
		r.Properties.Username = username
	}
	return nil
}

// OutputResources returns the output resources array.
func (r *RedisCache) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the application resource metadata.
func (r *RedisCache) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

func (redis *RedisCache) ResourceTypeName() string {
	return linkrp.RedisCachesResourceType
}

func (redis *RedisCache) Recipe() *linkrp.LinkRecipe {
	return &redis.Properties.Recipe
}

func (redisSecrets *RedisCacheSecrets) IsEmpty() bool {
	return redisSecrets == nil || *redisSecrets == RedisCacheSecrets{}
}

type RedisCacheProperties struct {
	rpv1.BasicResourceProperties
	Host          string                       `json:"host,omitempty"`
	Port          int32                        `json:"port,omitempty"`
	Username      string                       `json:"username,omitempty"`
	Recipe        linkrp.LinkRecipe            `json:"recipe,omitempty"`
	Secrets       RedisCacheSecrets            `json:"secrets,omitempty"`
	DisableRecipe bool                         `json:"disableRecipe,omitempty"`
	Resources     []linkrp.SupportingResources `json:"resources,omitempty"`
	Mode          LinkMode                     `json:"mode,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RedisCacheSecrets struct {
	ConnectionString string `json:"connectionString"`
	Password         string `json:"password"`
}

func (redis RedisCacheSecrets) ResourceTypeName() string {
	return linkrp.RedisCachesResourceType
}
