/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package datamodel

import (
	"errors"
	"fmt"
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

// Recipe returns the recipe for the Redis cache
func (redis *RedisCache) Recipe() *linkrp.LinkRecipe {
	if redis.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &redis.Properties.Recipe
}

func (redisSecrets *RedisCacheSecrets) IsEmpty() bool {
	return redisSecrets == nil || *redisSecrets == RedisCacheSecrets{}
}

// VerifyInputs checks that the inputs for manual resource provisioning are all provided
func (redisCache *RedisCache) VerifyInputs() error {
	if redisCache.Properties.ResourceProvisioning != "" && redisCache.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		if redisCache.Properties.Host == "" || redisCache.Properties.Port == 0 {
			return &v1.ErrClientRP{Code: "Bad Request", Message: fmt.Sprintf("host and port are required when resourceProvisioning is %s", linkrp.ResourceProvisioningManual)}
		}
	}
	return nil
}

type RedisCacheProperties struct {
	rpv1.BasicResourceProperties
	// The host name of the target Redis cache
	Host string `json:"host,omitempty"`

	// The port value of the target Redis cache
	Port int32 `json:"port,omitempty"`

	// The username for Redis cache
	Username string `json:"username,omitempty"`

	// The recipe used to automatically deploy underlying infrastructure for the Redis caches link
	Recipe linkrp.LinkRecipe `json:"recipe,omitempty"`

	// Secrets provided by resource
	Secrets RedisCacheSecrets `json:"secrets,omitempty"`

	// Specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning linkrp.ResourceProvisioning `json:"resourceProvisioning,omitempty"`

	// List of the resource IDs that support the Redis resource
	Resources []*linkrp.ResourceReference `json:"resources,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RedisCacheSecrets struct {
	ConnectionString string `json:"connectionString"`
	Password         string `json:"password"`
}

func (redis RedisCacheSecrets) ResourceTypeName() string {
	return linkrp.RedisCachesResourceType
}
