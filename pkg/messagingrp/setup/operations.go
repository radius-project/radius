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
		Name: "Applications.Messaging/rabbitMQQueues/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "rabbitMQQueues",
			Operation:   "List rabbitMQQueues",
			Description: "List RabbitMQ queue resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Messaging/rabbitMQQueues/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "rabbitMQQueues",
			Operation:   "Create/Update rabbitMQQueues",
			Description: "Create or update a RabbitMQ queue resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Messaging/rabbitMQQueues/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "rabbitMQQueues",
			Operation:   "Delete rabbitMQQueues",
			Description: "Delete a RabbitMQ queue resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Messaging/rabbitMQQueues/listsecrets/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "rabbitMQQueues",
			Operation:   "List secrets",
			Description: "Lists RabbitMQ queue secrets.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Messaging/register/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "Applications.Messaging",
			Operation:   "Register Applications.Messaging",
			Description: "Register the subscription for Applications.Messaging.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Messaging/unregister/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Messaging",
			Resource:    "Applications.Messaging",
			Operation:   "Unregister Applications.Messaging",
			Description: "Unregister the subscription for Applications.Messaging.",
		},
		IsDataAction: false,
	},
}
