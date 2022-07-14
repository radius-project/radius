// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/preview/sql/mgmt/2015-05-01-preview/sql"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var sqlServerDependency outputresource.Dependency = outputresource.Dependency{
	LocalID: outputresource.LocalIDAzureSqlServer,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

// Render creates the output resource for the sqlDatabase resource.
func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.SqlDatabase)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	properties := resource.Properties

	if resource.Properties.Resource == "" {
		if properties.Server == "" || properties.Database == "" {
			return renderers.RendererOutput{}, renderers.ErrorResourceOrServerNameMissingFromResource
		}
		return renderers.RendererOutput{
			Resources: []outputresource.OutputResource{},
			ComputedValues: map[string]renderers.ComputedValueReference{
				"database": {
					Value: properties.Database,
				},
				"server": {
					Value: properties.Server,
				},
			},
			// We don't provide any secret values here because SQL requires the USER to manage
			// the usernames and passwords. We don't have access!
			SecretValues: map[string]rp.SecretValueReference{},
		}, nil
	} else {
		// Source resource identifier is provided, currently only Azure resources are expected with non empty resource id
		rendererOutput, err := renderAzureResource(properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		return rendererOutput, nil
	}
}

func renderAzureResource(properties datamodel.SqlDatabaseProperties) (renderers.RendererOutput, error) {
	// Validate fully qualified resource identifier of the source resource is supplied for this connector
	databaseID, err := resources.Parse(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, errors.New("the 'resource' field must be a valid resource id")
	}
	// Validate resource type matches the expected Azure SQL DB resource type
	err = databaseID.ValidateResourceType(AzureSQLResourceType)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("the 'resource' field must refer to a %s", "SQL Database")
	}

	// Build output resources
	// Truncate the database part of the ID to get ID for the server
	serverID := databaseID.Truncate()

	serverResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureSqlServer,
		Provider: providers.ProviderAzure,
	}
	serverResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureSqlServer,
		ResourceType: serverResourceType,
		Identity:     resourcemodel.NewARMIdentity(&serverResourceType, serverID.String(), clients.GetAPIVersionFromUserAgent(sql.UserAgent())),
		Resource:     map[string]string{},
	}
	databaseResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureSqlServerDatabase,
		Provider: providers.ProviderAzure,
	}
	databaseResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureSqlServerDatabase,
		ResourceType: databaseResourceType,
		Identity:     resourcemodel.NewARMIdentity(&databaseResourceType, databaseID.String(), clients.GetAPIVersionFromUserAgent(sql.UserAgent())),
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
	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{serverResource, databaseResource},
		ComputedValues: computedValues,
		SecretValues:   map[string]rp.SecretValueReference{},
	}, nil
}
