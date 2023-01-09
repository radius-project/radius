// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/sql/mgmt/sql"
	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
				Type: "Applications.Link/sqlDatabases",
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
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

	require.Equal(t, outputresource.LocalIDAzureSqlServer, serverResource.LocalID)
	require.Equal(t, resourcekinds.AzureSqlServer, serverResource.ResourceType.Type)
	require.Equal(t, resourcemodel.NewARMIdentity(
		&resourcemodel.ResourceType{
			Type:     resourcekinds.AzureSqlServer,
			Provider: resourcemodel.ProviderAzure,
		},
		"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server",
		clients.GetAPIVersionFromUserAgent(sql.UserAgent())),
		serverResource.Identity)

	require.Equal(t, outputresource.LocalIDAzureSqlServerDatabase, databaseResource.LocalID)
	require.Equal(t, resourcekinds.AzureSqlServerDatabase, databaseResource.ResourceType.Type)
	require.Equal(t, resourcemodel.NewARMIdentity(
		&resourcemodel.ResourceType{
			Type:     resourcekinds.AzureSqlServerDatabase,
			Provider: resourcemodel.ProviderAzure,
		}, "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server/databases/test-database",
		clients.GetAPIVersionFromUserAgent(sql.UserAgent())),
		databaseResource.Identity)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: "test-database",
		},
		"server": {
			LocalID:     outputresource.LocalIDAzureSqlServer,
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
				Type: "Applications.Link/sqlDatabases",
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, renderers.ErrorResourceOrServerNameMissingFromResource.Error(), err.(*conv.ErrClientRP).Message)
}

func Test_Render_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}
	resource := datamodel.SqlDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/sqlDatabases/sql0",
				Name: "sql0",
				Type: "Applications.Link/sqlDatabases",
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/servers/test-database",
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must refer to an Azure SQL Database", err.(*conv.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}
	resource := datamodel.SqlDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/sqlDatabases/sql0",
				Name: "sql0",
				Type: "Applications.Link/sqlDatabases",
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "invalid-app-id",
			},
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server/databases/test-database",
		},
	}

	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*conv.ErrClientRP).Message)
}
