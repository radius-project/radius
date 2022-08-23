// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/armerrors"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"
)

const (
	userName         = "admin"
	password         = "testpassword"
	connectionString = "test-connection-string"
)

func Test_Render_Success(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/mongoDatabases/mongo0",
			Name: "mongo0",
			Type: "Applications.Connector/mongoDatabases",
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: v1.BasicResourceProperties{
					Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	output, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.NoError(t, err)

	require.Len(t, output.Resources, 2)
	accountResource := output.Resources[0]
	databaseResource := output.Resources[1]

	require.Equal(t, outputresource.LocalIDAzureCosmosAccount, accountResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosAccount, accountResource.ResourceType.Type)

	require.Equal(t, outputresource.LocalIDAzureCosmosDBMongo, databaseResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosDBMongo, databaseResource.ResourceType.Type)

	expectedAccount := map[string]string{
		handlers.CosmosDBAccountIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
		handlers.CosmosDBAccountNameKey: "test-account",
		handlers.CosmosDBAccountKindKey: string(documentdb.DatabaseAccountKindMongoDB),
	}
	require.Equal(t, expectedAccount, accountResource.Resource)

	expectedDatabase := map[string]string{
		handlers.CosmosDBAccountIDKey:    "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
		handlers.CosmosDBAccountNameKey:  "test-account",
		handlers.CosmosDBDatabaseIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
		handlers.CosmosDBDatabaseNameKey: "test-database",
	}
	require.Equal(t, expectedDatabase, databaseResource.Resource)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.DatabaseNameValue: {
			Value: "test-database",
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, "/connectionStrings/0/connectionString", output.SecretValues[renderers.ConnectionStringValue].ValueSelector)
	require.Equal(t, "listConnectionStrings", output.SecretValues[renderers.ConnectionStringValue].Action)
}

func Test_Render_UserSpecifiedSecrets(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/mongoDatabases/mongo0",
			Name: "mongo0",
			Type: "Applications.Connector/mongoDatabases",
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: v1.BasicResourceProperties{
					Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
			},
			Secrets: datamodel.MongoDatabaseSecrets{
				Username:         userName,
				Password:         password,
				ConnectionString: connectionString,
			},
		},
	}

	output, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.NoError(t, err)
	require.Len(t, output.Resources, 0)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.DatabaseNameValue: {
			Value: mongoDBResource.Name,
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	expectedSecretValues := map[string]rp.SecretValueReference{
		renderers.ConnectionStringValue: {Value: connectionString},
		renderers.UsernameStringValue:   {Value: userName},
		renderers.PasswordStringHolder:  {Value: password},
	}
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Render_NoResourceSpecified(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/mongoDatabases/mongo0",
			Name: "mongo0",
			Type: "Applications.Connector/mongoDatabases",
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: v1.BasicResourceProperties{
					Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
			},
		},
	}

	output, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, len(output.Resources))
}

func Test_Render_InvalidResourceModel(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.SqlDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/mongoDatabases/mongo0",
			Name: "mongo0",
			Type: "Applications.Connector/mongoDatabases",
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: v1.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, "invalid model conversion", err.Error())
}

func Test_Render_InvalidSourceResourceIdentifier(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/mongoDatabases/mongo0",
			Name: "mongo0",
			Type: "Applications.Connector/mongoDatabases",
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: v1.BasicResourceProperties{
					Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, armerrors.Invalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must be a valid resource id", err.(*conv.ErrClientRP).Message)
}

func Test_Render_InvalidResourceType(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/mongoDatabases/mongo0",
			Name: "mongo0",
			Type: "Applications.Connector/mongoDatabases",
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: v1.BasicResourceProperties{
					Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, armerrors.Invalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must refer to an Azure CosmosDB Mongo Database resource", err.(*conv.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/mongoDatabases/mongo0",
			Name: "mongo0",
			Type: "Applications.Connector/mongoDatabases",
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: v1.BasicResourceProperties{
					Application: "invalid-app-id",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, armerrors.Invalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*conv.ErrClientRP).Message)
}
