// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package microsoftsqlv1alpha3

import (
	"context"
	"errors"

	"github.com/Azure/azure-sdk-for-go/services/preview/sql/mgmt/2015-05-01-preview/sql"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
)

var sqlServerDependency outputresource.Dependency = outputresource.Dependency{
	LocalID: outputresource.LocalIDAzureSqlServer,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := MicrosoftSQLComponentProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if properties.Managed {
		return renderers.RendererOutput{}, errors.New("only 'managed: true' SQL components are supported")
	}

	if properties.Resource == "" {
		return renderers.RendererOutput{}, renderers.ErrResourceMissingForUnmanagedResource
	}

	databaseID, err := renderers.ValidateResourceID(properties.Resource, SQLResourceType, "SQL Database")
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
