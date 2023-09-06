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
	"fmt"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/portableresources"
	pr_dm "github.com/radius-project/radius/pkg/portableresources/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

// RedisCache represents Redis cache portable resource.
type RedisCache struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RedisCacheProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	pr_dm.LinkMetadata
}

// ApplyDeploymentOutput sets the Status, ComputedValues, SecretValues, Host, Port and Username properties of the
// Redis cache instance based on the DeploymentOutput object.
func (r *RedisCache) ApplyDeploymentOutput(do rpv1.DeploymentOutput) error {
	return nil
}

// OutputResources returns the OutputResources of the Redis cache resource.
func (r *RedisCache) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// ResourceMetadata returns the BasicResourceProperties of the Redis cache resource.
func (r *RedisCache) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// ResourceTypeName returns the resource type of Redis cache resource.
func (redis *RedisCache) ResourceTypeName() string {
	return portableresources.RedisCachesResourceType
}

// Recipe returns the LinkRecipe from the Redis cache Properties if ResourceProvisioning is not set to Manual,
// otherwise it returns nil.
func (redis *RedisCache) Recipe() *portableresources.LinkRecipe {
	if redis.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		return nil
	}
	return &redis.Properties.Recipe
}

// IsEmpty checks if the RedisCacheSecrets instance is empty or not.
func (redisSecrets *RedisCacheSecrets) IsEmpty() bool {
	return redisSecrets == nil || *redisSecrets == RedisCacheSecrets{}
}

// VerifyInputs checks if the required fields are set when the resourceProvisioning is set to manual
// and returns an error if not.
func (redisCache *RedisCache) VerifyInputs() error {
	msgs := []string{}
	if redisCache.Properties.ResourceProvisioning != "" && redisCache.Properties.ResourceProvisioning == portableresources.ResourceProvisioningManual {
		if redisCache.Properties.Host == "" {
			msgs = append(msgs, "host must be specified when resourceProvisioning is set to manual")
		}
		if redisCache.Properties.Port == 0 {
			msgs = append(msgs, "port must be specified when resourceProvisioning is set to manual")
		}
	}

	if len(msgs) == 1 {
		return &v1.ErrClientRP{
			Code:    v1.CodeInvalid,
			Message: msgs[0],
		}
	} else if len(msgs) > 1 {
		return &v1.ErrClientRP{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("multiple errors were found:\n\t%v", strings.Join(msgs, "\n\t")),
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

	// Specifies whether to enable non-SSL or SSL connections
	TLS bool `json:"tls,omitempty"`

	// The recipe used to automatically deploy underlying infrastructure for the Redis caches link
	Recipe portableresources.LinkRecipe `json:"recipe,omitempty"`

	// Secrets provided by resource
	Secrets RedisCacheSecrets `json:"secrets,omitempty"`

	// Specifies how the underlying service/resource is provisioned and managed
	ResourceProvisioning portableresources.ResourceProvisioning `json:"resourceProvisioning,omitempty"`

	// List of the resource IDs that support the Redis resource
	Resources []*portableresources.ResourceReference `json:"resources,omitempty"`
}

// Secrets values consisting of secrets provided for the resource
type RedisCacheSecrets struct {
	ConnectionString string `json:"connectionString"`
	Password         string `json:"password"`
	URL              string `json:"url"`
}

// ResourceTypeName returns the resource type of RedisCache resource.
func (redis RedisCacheSecrets) ResourceTypeName() string {
	return portableresources.RedisCachesResourceType
}
