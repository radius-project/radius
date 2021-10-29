// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package microsoftsqlv1alpha3

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/preview/sql/mgmt/2015-05-01-preview/sql"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

const (
	applicationName = "test-app"
	resourceName    = "test-db"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_Unmanaged_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server/databases/test-database",
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 2)
	accountResource := output.Resources[0]
	databaseResource := output.Resources[1]

	require.Equal(t, outputresource.LocalIDAzureSqlServer, accountResource.LocalID)
	require.Equal(t, resourcekinds.AzureSqlServer, accountResource.ResourceKind)
	require.Equal(t, resourcemodel.NewARMIdentity("/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server", clients.GetAPIVersionFromUserAgent(sql.UserAgent())), accountResource.Identity)

	require.Equal(t, outputresource.LocalIDAzureSqlServerDatabase, databaseResource.LocalID)
	require.Equal(t, resourcekinds.AzureSqlServerDatabase, databaseResource.ResourceKind)
	require.Equal(t, resourcemodel.NewARMIdentity("/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Sql/servers/test-server/databases/test-database", clients.GetAPIVersionFromUserAgent(sql.UserAgent())), databaseResource.Identity)

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

func Test_Render_Unmanaged_MissingResource(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": false,
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, renderers.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
}

func Test_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/servers/sqlDatabases/test-database",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a SQL Database", err.Error())
}
