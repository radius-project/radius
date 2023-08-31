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

package connections

import (
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
)

// This file contains shared variables and functions used in tests.

var environmentResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
var applicationResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
var containerResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/webapp"
var redisResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/redis"

var awsMemoryDBResourceID = "/planes/aws/aws/accounts/00000000/regions/us-west-2/providers/AWS.MemoryDB/Cluster/redis-aqbjixghynqgg"
var azureRedisCacheResourceID = "/planes/azure/azure/subscriptions/00000000/resourceGroups/azure-group/providers/Microsoft.Cache/Redis/redis"

func makeRedisResourceID(name string) string {
	return "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/" + name
}

var containerDeploymentOutputResource any = makeKubernetesOutputResource("apps", "Deployment", "default-demo", "demo")
var redisAWSOutputResource any = makeOutputResource(awsMemoryDBResourceID)
var redisAzureOutputResource any = makeOutputResource(azureRedisCacheResourceID)

// makeKubernetesOutputResource creates a Kubernetes output resource.
func makeKubernetesOutputResource(group string, kind string, namespace string, name string) map[string]any {
	return map[string]any{
		"id": resources_kubernetes.IDFromParts(resources_kubernetes.PlaneNameTODO, group, kind, namespace, name),
	}
}

// makeOutputResource creates an AWS output resource.
func makeOutputResource(id string) map[string]any {
	return map[string]any{
		"id": id,
	}
}

// makeResourceProperties creates a map of resource properties for a resource.
//
// connections should contain a map of name -> resource ID.
// outputResources should contain the list of output resources.
func makeResourceProperties(connections map[string]string, outputResources []any) map[string]any {
	properties := map[string]any{}

	if connections != nil {
		c := map[string]any{}
		for name, id := range connections {
			c[name] = map[string]any{
				"source": id,
			}
		}
		properties["connections"] = c
	}

	if len(outputResources) > 0 {
		status := map[string]any{
			"outputResources": outputResources,
		}
		properties["status"] = status
	}

	return properties
}
