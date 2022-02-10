// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package microsoftsqlv1alpha3

import (
	"context"
	"errors"

	"github.com/Azure/azure-sdk-for-go/services/preview/sql/mgmt/2015-05-01-preview/sql"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

var sqlServerDependency outputresource.Dependency = outputresource.Dependency{
	LocalID: outputresource.LocalIDAzureSqlServer,
}

var ErrorResourceOrServerNameMissingFromUnmanagedResource = errors.New("either the 'resource' or 'server'/'database' is required when 'managed' is not specified")

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	Kubernetes bool
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.MicrosoftSQLDatabaseProperties{}
	resource := options.Resource
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if properties.Resource == nil || *properties.Resource == "" {
		// Server and database names are required if no resource id
		if properties.Server == nil || *properties.Server == "" {
			return renderers.RendererOutput{}, ErrorResourceOrServerNameMissingFromUnmanagedResource
		}

		if properties.Database == nil || *properties.Database == "" {
			return renderers.RendererOutput{}, ErrorResourceOrServerNameMissingFromUnmanagedResource
		}

		computedValues := map[string]renderers.ComputedValueReference{
			"database": {
				Value: *properties.Database,
			},
			"server": {
				Value: *properties.Server,
			},
		}

		// We don't provide any secret values here because SQL requires the USER to manage
		// the usernames and passwords. We don't have access!
		secretValues := map[string]renderers.SecretValueReference{}
		return renderers.RendererOutput{
			Resources:      []outputresource.OutputResource{},
			ComputedValues: computedValues,
			SecretValues:   secretValues,
		}, nil
	} else {
		if r.Kubernetes {
			return renderers.RendererOutput{}, errors.New("cannot reference resourceID on Kubernetes")
		}

		databaseID, err := renderers.ValidateResourceID(*properties.Resource, SQLResourceType, "SQL Database")
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		// Truncate the database part of the ID to make an ID for the server
		serverID := databaseID.Truncate()

		serverResource := outputresource.OutputResource{
			LocalID:      outputresource.LocalIDAzureSqlServer,
			ResourceKind: resourcekinds.AzureSqlServer,
			Identity:     resourcemodel.NewARMIdentity(serverID.ID, clients.GetAPIVersionFromUserAgent(sql.UserAgent())),
			Resource:     map[string]string{},
		}

		databaseResource := outputresource.OutputResource{
			LocalID:      outputresource.LocalIDAzureSqlServerDatabase,
			ResourceKind: resourcekinds.AzureSqlServerDatabase,
			Identity:     resourcemodel.NewARMIdentity(databaseID.ID, clients.GetAPIVersionFromUserAgent(sql.UserAgent())),
			Resource:     map[string]string{},
			Dependencies: []outputresource.Dependency{sqlServerDependency},
		}

		computedValues := map[string]renderers.ComputedValueReference{
			"database": {
				Value: databaseID.Name(),
			},
			"server": {
				LocalID:     outputresource.LocalIDAzureSqlServer,
				JSONPointer: "/properties/fullyQualifiedDomainName",
			},
		}

		// We don't provide any secret values here because SQL requires the USER to manage
		// the usernames and passwords. We don't have access!
		secretValues := map[string]renderers.SecretValueReference{}

		return renderers.RendererOutput{
			Resources:      []outputresource.OutputResource{serverResource, databaseResource},
			ComputedValues: computedValues,
			SecretValues:   secretValues,
		}, nil
	}

}
