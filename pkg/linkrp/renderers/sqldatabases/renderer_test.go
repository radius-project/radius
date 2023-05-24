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

package sqldatabases

import (
	"context"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := datamodel.SqlDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/sqlDatabases/sql0",
				Name: "sql0",
				Type: linkrp.SqlDatabasesResourceType,
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server/databases/test-database",
		},
	}

	output, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.NoError(t, err)

	require.Len(t, output.Resources, 2)
	serverResource := output.Resources[0]
	databaseResource := output.Resources[1]

	require.Equal(t, rpv1.LocalIDAzureSqlServer, serverResource.LocalID)
	require.Equal(t, resourcekinds.AzureSqlServer, serverResource.ResourceType.Type)
	require.Equal(t, resourcemodel.NewARMIdentity(
		&resourcemodel.ResourceType{
			Type:     resourcekinds.AzureSqlServer,
			Provider: resourcemodel.ProviderAzure,
		},
		"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server",
		clientv2.SQLManagementClientAPIVersion),
		serverResource.Identity)

	require.Equal(t, rpv1.LocalIDAzureSqlServerDatabase, databaseResource.LocalID)
	require.Equal(t, resourcekinds.AzureSqlServerDatabase, databaseResource.ResourceType.Type)
	require.Equal(t, resourcemodel.NewARMIdentity(
		&resourcemodel.ResourceType{
			Type:     resourcekinds.AzureSqlServerDatabase,
			Provider: resourcemodel.ProviderAzure,
		}, "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server/databases/test-database",
		clientv2.SQLManagementClientAPIVersion),
		databaseResource.Identity)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: "test-database",
		},
		"server": {
			LocalID:     rpv1.LocalIDAzureSqlServer,
			JSONPointer: "/properties/fullyQualifiedDomainName",
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Empty(t, output.SecretValues)
}

func Test_Render_MissingResource(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := datamodel.SqlDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/sqlDatabases/sql0",
				Name: "sql0",
				Type: linkrp.SqlDatabasesResourceType,
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, renderers.ErrorResourceOrServerNameMissingFromResource.Error(), err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}
	resource := datamodel.SqlDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/sqlDatabases/sql0",
				Name: "sql0",
				Type: linkrp.SqlDatabasesResourceType,
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/servers/test-database",
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must refer to an Azure SQL Database", err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}
	resource := datamodel.SqlDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/sqlDatabases/sql0",
				Name: "sql0",
				Type: linkrp.SqlDatabasesResourceType,
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "invalid-app-id",
			},
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server/databases/test-database",
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
}
