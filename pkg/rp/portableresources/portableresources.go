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

package portableresources

import (
	"strings"

	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	msg_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller"
)

const ExtendersResourceType = "Applications.Core/extenders"

// IsValidPortableResourceType checks if the provided resource type is a valid portable resource type.
// Returns true if the resource type is valid, false otherwise.
func IsValidPortableResourceType(resourceType string) bool {
	portableResourceTypes := []string{
		dapr_ctrl.DaprPubSubBrokersResourceType,
		dapr_ctrl.DaprSecretStoresResourceType,
		dapr_ctrl.DaprStateStoresResourceType,
		msg_ctrl.RabbitMQQueuesResourceType,
		ds_ctrl.MongoDatabasesResourceType,
		ds_ctrl.RedisCachesResourceType,
		ds_ctrl.SqlDatabasesResourceType,
		ExtendersResourceType,
	}

	for _, s := range portableResourceTypes {
		if strings.EqualFold(s, resourceType) {
			return true
		}
	}

	return false
}
