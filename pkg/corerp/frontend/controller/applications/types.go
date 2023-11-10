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

package applications

import (
	cntr_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/containers"
	ext_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/extenders"
	gtwy_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/gateways"
	hrt_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/httproutes"
	sstr_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/secretstores"
	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	msg_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller"
)

const (
	ResourceTypeName = "Applications.Core/applications"
)

var (
	resourceTypesList = []string{
		ds_ctrl.MongoDatabasesResourceType,
		msg_ctrl.RabbitMQQueuesResourceType,
		ds_ctrl.RedisCachesResourceType,
		ds_ctrl.SqlDatabasesResourceType,
		dapr_ctrl.DaprStateStoresResourceType,
		dapr_ctrl.DaprSecretStoresResourceType,
		dapr_ctrl.DaprPubSubBrokersResourceType,
		ext_ctrl.ResourceTypeName,
		gtwy_ctrl.ResourceTypeName,
		hrt_ctrl.ResourceTypeName,
		cntr_ctrl.ResourceTypeName,
		sstr_ctrl.ResourceTypeName,
	}
)
