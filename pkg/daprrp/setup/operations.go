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
		Name: "Applications.Dapr/operations/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "operations",
			Operation:   "Get operations",
			Description: "Get the list of operations.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/register/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "Applications.Dapr",
			Operation:   "Register Applications.Dapr resource provider",
			Description: "Registers 'Applications.Dapr' resource provider with a subscription.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/unregister/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "Applications.Dapr",
			Operation:   "Unregister 'Applications.Dapr' resource provider",
			Description: "Unregisters 'Applications.Dapr' resource provider with a subscription.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/secretStores/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "secretStores",
			Operation:   "Get/List daprSecretStores",
			Description: "Gets/Lists daprSecretStore resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/secretStores/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "secretStores",
			Operation:   "Create/Update daprSecretStores",
			Description: "Creates or updates a daprSecretStore resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/secretStores/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "secretStores",
			Operation:   "Delete daprSecretStore",
			Description: "Deletes a daprSecretStore resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/stateStores/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "stateStores",
			Operation:   "Get/List daprStateStores",
			Description: "Gets/Lists daprStateStore resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/stateStores/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "stateStores",
			Operation:   "Create/Update daprStateStores",
			Description: "Creates or updates a daprStateStore resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/stateStores/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "stateStores",
			Operation:   "Delete daprStateStore",
			Description: "Deletes a daprStateStore resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/pubSubBrokers/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "pubSubBrokers",
			Operation:   "Get/List daprPubSubBrokers",
			Description: "Gets/Lists daprPubSubBroker resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/pubSubBrokers/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "pubSubBrokers",
			Operation:   "Create/Update daprPubSubBrokers",
			Description: "Creates or updates a daprPubSubBroker resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/pubSubBrokers/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "pubSubBrokers",
			Operation:   "Delete daprPubSubBroker",
			Description: "Deletes a daprPubSubBroker resource.",
		},
		IsDataAction: false,
	},
}
