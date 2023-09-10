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
		Name: "Applications.Core/operations/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "operations",
			Operation:   "Get operations",
			Description: "Get the list of operations",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/environments/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "environments",
			Operation:   "List environments",
			Description: "Get the list of environments.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/environments/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "environments",
			Operation:   "Create/Update environment",
			Description: "Create or update an environment.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/environments/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "environments",
			Operation:   "Delete environment",
			Description: "Delete an environment.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/environments/getmetadata/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "environments",
			Operation:   "Get recipe metadata",
			Description: "Get recipe metadata.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/environments/join/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "environments",
			Operation:   "Join environment",
			Description: "Join to application environment.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/register/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "Applications.Core",
			Operation:   "Register Applications.Core",
			Description: "Register the subscription for Applications.Core.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/unregister/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "Applications.Core",
			Operation:   "Unregister Applications.Core",
			Description: "Unregister the subscription for Applications.Core.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/httproutes/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "httproutes",
			Operation:   "List httproutes",
			Description: "Get the list of httproutes.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/httproutes/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "httproutes",
			Operation:   "Create/Update httproute",
			Description: "Create or update an httproute.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/httproutes/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "httproutes",
			Operation:   "Delete httproute",
			Description: "Delete an httproute.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/applications/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "applications",
			Operation:   "List applications",
			Description: "Get the list of applications.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/applications/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "applications",
			Operation:   "Create/Update application",
			Description: "Create or update an application.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/applications/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "applications",
			Operation:   "Delete application",
			Description: "Delete an application.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/gateways/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "gateways",
			Operation:   "List gateways",
			Description: "Get the list of gateways.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/gateways/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "gateways",
			Operation:   "Create/Update gateway",
			Description: "Create or Update a gateway.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/gateways/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "gateways",
			Operation:   "delete gateway",
			Description: "Delete a gateway.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/containers/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "containers",
			Operation:   "List containers",
			Description: "Get the list of containers.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/containers/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "containers",
			Operation:   "Create/Update container",
			Description: "Create or update a container.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/containers/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "containers",
			Operation:   "Delete container",
			Description: "Delete a container.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/extenders/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "extenders",
			Operation:   "Get/List extenders",
			Description: "Gets/Lists extender link(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/extenders/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "extenders",
			Operation:   "Create/Update extenders",
			Description: "Creates or updates a extender resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/extenders/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "extenders",
			Operation:   "Delete extender",
			Description: "Deletes a extender resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Core/extenders/listsecrets/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Core",
			Resource:    "extenders",
			Operation:   "List secrets",
			Description: "Lists extender secrets.",
		},
		IsDataAction: false,
	},
}
