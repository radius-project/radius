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
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
)

// This file contains shared variables, constants and functions used in tests.
const containerResourceType = "Applications.Core/containers"
const redisResourceType = "Applications.Datastores/redisCaches"
const provisioningStateSuccess = "Success"

var directionOutbound = corerpv20231001preview.DirectionOutbound
var directionInbound = corerpv20231001preview.DirectionInbound
var environmentResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
var applicationResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
var containerResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/containers/webapp"
var redisResourceID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/redis"
var containerResourceName = "webapp"
var redisResourceName = "redis"

var awsMemoryDBResourceID = "/planes/aws/aws/accounts/00000000/regions/us-west-2/providers/AWS.MemoryDB/Cluster/redis-aqbjixghynqgg"
var azureRedisCacheResourceID = "/planes/azure/azure/subscriptions/00000000/resourceGroups/azure-group/providers/Microsoft.Cache/Redis/redis"
