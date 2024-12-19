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
			Operation:   "Get/List Dapr secretStores",
			Description: "Gets/Lists Dapr secretStore resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/secretStores/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "secretStores",
			Operation:   "Create/Update Dapr secretStores",
			Description: "Creates or updates a Dapr secretStore resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/secretStores/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "secretStores",
			Operation:   "Delete Dapr secretStore",
			Description: "Deletes a Dapr secretStore resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/stateStores/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "stateStores",
			Operation:   "Get/List Dapr stateStores",
			Description: "Gets/Lists Dapr stateStore resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/stateStores/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "stateStores",
			Operation:   "Create/Update Dapr stateStores",
			Description: "Creates or updates a Dapr stateStore resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/stateStores/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "stateStores",
			Operation:   "Delete Dapr stateStore",
			Description: "Deletes a Dapr stateStore resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/pubSubBrokers/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "pubSubBrokers",
			Operation:   "Get/List Dapr pubSubBrokers",
			Description: "Gets/Lists Dapr pubSubBroker resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/pubSubBrokers/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "pubSubBrokers",
			Operation:   "Create/Update Dapr pubSubBrokers",
			Description: "Creates or updates a Dapr pubSubBroker resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/pubSubBrokers/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "pubSubBrokers",
			Operation:   "Delete Dapr pubSubBroker",
			Description: "Deletes a Dapr pubSubBroker resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/configurationStores/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "configurationStores",
			Operation:   "Get/List Dapr configurationStores",
			Description: "Gets/Lists Dapr configurationStores resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/configurationStores/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "configurationStores",
			Operation:   "Create/Update Dapr configurationStores",
			Description: "Creates or updates a Dapr configurationStores resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/configurationStores/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "configurationStores",
			Operation:   "Delete Dapr configurationStores",
			Description: "Deletes a Dapr configurationStores resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/bindings/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "bindings",
			Operation:   "Get/List Dapr bindings",
			Description: "Gets/Lists Dapr bindings resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/bindings/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "bindings",
			Operation:   "Create/Update Dapr bindings",
			Description: "Creates or updates a Dapr bindings resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Dapr/bindings/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Dapr",
			Resource:    "bindings",
			Operation:   "Delete Dapr bindings",
			Description: "Deletes a Dapr bindings resource.",
		},
		IsDataAction: false,
	},
}
