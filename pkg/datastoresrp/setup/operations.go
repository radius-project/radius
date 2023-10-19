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
		Name: "Applications.Datastores/operations/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "operations",
			Operation:   "Get operations",
			Description: "Get the list of operations",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/redisCaches/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "redisCaches",
			Operation:   "List redisCaches",
			Description: "List Redis cache resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/redisCaches/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "redisCaches",
			Operation:   "Create/Update redisCaches",
			Description: "Create or update an Redis cache resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/redisCaches/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "redisCaches",
			Operation:   "Delete redisCache",
			Description: "Delete a Redis cache resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/redisCaches/getmetadata/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "redisCaches",
			Operation:   "Get recipe metadata",
			Description: "Get recipe metadata.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/redisCaches/listsecrets/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "redisCaches",
			Operation:   "List secrets",
			Description: "Lists Redis cache secrets.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/register/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "Applications.Datastores",
			Operation:   "Register Applications.Datastores",
			Description: "Register the subscription for Applications.Datastores.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/unregister/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "Applications.Datastores",
			Operation:   "Unregister Applications.Datastores",
			Description: "Unregister the subscription for Applications.Datastores.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/mongoDatabases/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "mongoDatabases",
			Operation:   "List mongoDatabases",
			Description: "List Mongo database resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/mongoDatabases/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "mongoDatabases",
			Operation:   "Create/Update mongoDatabases",
			Description: "Create or update a Mongo database resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/mongoDatabases/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "mongoDatabases",
			Operation:   "Delete mongoDatabases",
			Description: "Delete a Mongo databases resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/mongoDatabases/listsecrets/action",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastore",
			Resource:    "mongoDatabases",
			Operation:   "List secrets",
			Description: "List secret(s) of Mongo database resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/sqlDatabases/read",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "sqlDatabases",
			Operation:   "List sqlDatabases",
			Description: "List SQL database resource(s).",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/sqlDatabases/write",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "sqlDatabases",
			Operation:   "Create/Update sqlDatabases",
			Description: "Create or update a SQL database resource.",
		},
		IsDataAction: false,
	},
	{
		Name: "Applications.Datastores/sqlDatabases/delete",
		Display: &v1.OperationDisplayProperties{
			Provider:    "Applications.Datastores",
			Resource:    "sqlDatabases",
			Operation:   "Delete sqlDatabases",
			Description: "Delete a SQL database resource.",
		},
		IsDataAction: false,
	},
}
