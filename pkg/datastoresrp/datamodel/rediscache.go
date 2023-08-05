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
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	linkrp_dm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
)

// RedisCache represents RedisCache link resource.
type RedisCache struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties RedisCacheProperties `json:"properties"`

	// LinkMetadata represents internal DataModel properties common to all link types.
	linkrp_dm.LinkMetadata
}

// # Function Explanation
//
// ApplyDeploymentOutput sets the Status, ComputedValues, SecretValues, Host, Port and Username properties of the
// RedisCache instance based on the DeploymentOutput object.
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

// # Function Explanation
//
// OutputResources returns the OutputResources of the RedisCache resource.
func (r *RedisCache) OutputResources() []rpv1.OutputResource {
	return r.Properties.Status.OutputResources
}

// # Function Explanation
//
// ResourceMetadata returns the BasicResourceProperties of the RedisCache resource.
func (r *RedisCache) ResourceMetadata() *rpv1.BasicResourceProperties {
	return &r.Properties.BasicResourceProperties
}

// # Function Explanation
//
// ResourceTypeName returns the resource type of RedisCache resource.
func (redis *RedisCache) ResourceTypeName() string {
	return linkrp.N_RedisCachesResourceType
}

// # Function Explanation
//
// Recipe returns the LinkRecipe from the RedisCache Properties if ResourceProvisioning is not set to Manual,
// otherwise it returns nil.
func (redis *RedisCache) Recipe() *linkrp.LinkRecipe {
	if redis.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
		return nil
	}
	return &redis.Properties.Recipe
}

// # Function Explanation
//
// IsEmpty checks if the RedisCacheSecrets instance is empty or not.
func (redisSecrets *RedisCacheSecrets) IsEmpty() bool {
	return redisSecrets == nil || *redisSecrets == RedisCacheSecrets{}
}

// # Function Explanation
//
// VerifyInputs checks if the required fields are set when the resourceProvisioning is set to manual
// and returns an error if not.
func (redisCache *RedisCache) VerifyInputs() error {
	msgs := []string{}
	if redisCache.Properties.ResourceProvisioning != "" && redisCache.Properties.ResourceProvisioning == linkrp.ResourceProvisioningManual {
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
	URL              string `json:"url"`
}

// # Function Explanation
//
// ResourceTypeName returns the resource type of RedisCache resource.
func (redis RedisCacheSecrets) ResourceTypeName() string {
	return linkrp.N_RedisCachesResourceType
}
