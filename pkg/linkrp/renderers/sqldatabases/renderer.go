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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var sqlServerDependency rpv1.Dependency = rpv1.Dependency{
	LocalID: rpv1.LocalIDAzureSqlServer,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

// Render creates the output resource for the sqlDatabase resource.
func (r Renderer) Render(ctx context.Context, dm v1.ResourceDataModel, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.SqlDatabase)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}
	properties := resource.Properties

	_, err := renderers.ValidateApplicationID(properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	if resource.Properties.Resource == "" {
		if properties.Server == "" || properties.Database == "" {
			return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(renderers.ErrorResourceOrServerNameMissingFromResource.Error())
		}
		return renderers.RendererOutput{
			Resources: []rpv1.OutputResource{},
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
			SecretValues: map[string]rpv1.SecretValueReference{},
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
	// Validate fully qualified resource identifier of the source resource is supplied for this link
	databaseID, err := resources.ParseResource(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
	}
	// Validate resource type matches the expected Azure SQL DB resource type
	err = databaseID.ValidateResourceType(AzureSQLResourceType)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("the 'resource' field must refer to an Azure SQL Database")
	}

	// Build output resources
	// Truncate the database part of the ID to get ID for the server
	serverID := databaseID.Truncate()

	serverResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureSqlServer,
		Provider: resourcemodel.ProviderAzure,
	}
	serverResource := rpv1.OutputResource{
		LocalID:      rpv1.LocalIDAzureSqlServer,
		ResourceType: serverResourceType,
		Identity:     resourcemodel.NewARMIdentity(&serverResourceType, serverID.String(), clientv2.SQLManagementClientAPIVersion),
		Resource:     map[string]string{},
	}
	databaseResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureSqlServerDatabase,
		Provider: resourcemodel.ProviderAzure,
	}
	databaseResource := rpv1.OutputResource{
		LocalID:      rpv1.LocalIDAzureSqlServerDatabase,
		ResourceType: databaseResourceType,
		Identity:     resourcemodel.NewARMIdentity(&databaseResourceType, databaseID.String(), clientv2.SQLManagementClientAPIVersion),
		Resource:     map[string]string{},
		Dependencies: []rpv1.Dependency{sqlServerDependency},
	}

	computedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: databaseID.Name(),
		},
		"server": {
			LocalID:     rpv1.LocalIDAzureSqlServer,
			JSONPointer: "/properties/fullyQualifiedDomainName",
		},
	}

	// We don't provide any secret values here because SQL requires the USER to manage
	// the usernames and passwords. We don't have access!
	return renderers.RendererOutput{
		Resources:      []rpv1.OutputResource{serverResource, databaseResource},
		ComputedValues: computedValues,
		SecretValues:   map[string]rpv1.SecretValueReference{},
	}, nil
}
