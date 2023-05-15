// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

// Processor is a processor for MongoDB resources.
type Processor struct {
}

// Process implements the processors.Processor interface for MongoDB resources.
func (p *Processor) Process(ctx context.Context, resource *datamodel.MongoDatabase, options processors.Options) error {
	validator := processors.NewValidator(&resource.ComputedValues, &resource.SecretValues, &resource.Properties.Status.OutputResources)

	validator.AddResourcesField(&resource.Properties.Resources)
	validator.AddOptionalStringField(renderers.Host, &resource.Properties.Host)
	validator.AddOptionalInt32Field(renderers.Port, &resource.Properties.Port)
	validator.AddOptionalStringField(renderers.DatabaseNameValue, &resource.Properties.Database)
	validator.AddOptionalSecretField(renderers.UsernameStringValue, &resource.Properties.Secrets.Username)
	validator.AddOptionalSecretField(renderers.PasswordStringHolder, &resource.Properties.Secrets.Password)
	validator.AddComputedSecretField(renderers.ConnectionStringValue, &resource.Properties.Secrets.ConnectionString, func() (string, *processors.ValidationError) {
		return p.computeConnectionString(resource), nil
	})

	err := validator.SetAndValidate(options.RecipeOutput)
	if err != nil {
		return err
	}

	return nil
}

func (p *Processor) computeConnectionString(resource *datamodel.MongoDatabase) string {
	connectionString := "mongodb://"

	if resource.Properties.Secrets.Username != "" {
		connectionString += resource.Properties.Secrets.Username + ":"
	}
	if resource.Properties.Secrets.Password != "" {
		connectionString += resource.Properties.Secrets.Password + "@"
	}
	connectionString = fmt.Sprintf("%s%s:%v", connectionString, resource.Properties.Host, resource.Properties.Port)

	if resource.Properties.Database != "" {
		connectionString = connectionString + "/" + resource.Properties.Database
	}
	return connectionString
}
