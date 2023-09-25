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

package setup

import v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"

var operationList = []v1.Operation{
	{
		Name: "Applications.Messaging/operations/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "operations",
			Operation:   "Get operations",
			Description: "Get the list of operations",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Messaging/rabbitmqqueues/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "rabbitmqqueues",
			Operation:   "List RabbitMQ",
			Description: "Get the list of RabbitMQ.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Messaging/rabbitmqqueues/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "rabbitmqqueues",
			Operation:   "Create/Update RabbitMQ",
			Description: "Create or update RabbitMQ.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Messaging/rabbitmqqueues/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "rabbitmqqueues",
			Operation:   "Delete RabbitMQ",
			Description: "Delete RabbitMQ.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Messaging/rabbitmqqueues/listsecrets/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "rabbitmqqueues",
			Operation:   "List secrets",
			Description: "Lists RabbitMQ secrets.",
		},
		IsDataAction: false,
	},
}
