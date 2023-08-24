// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

// Processor is a processor for SQL database resources.
type Processor struct {
}

// Process implements the processors.Processor interface for SQL database resources. It validates the given resource properties
// and sets the computed values and secrets in the resource, and applies the values from the RecipeOutput.
func (p *Processor) Process(ctx context.Context, resource *datamodel.SqlDatabase, options processors.Options) error {
	validator := processors.NewValidator(&resource.ComputedValues, &resource.SecretValues, &resource.Properties.Status.OutputResources)

	validator.AddResourcesField(&resource.Properties.Resources)
	validator.AddRequiredStringField(renderers.DatabaseNameValue, &resource.Properties.Database)
	validator.AddRequiredStringField(renderers.ServerNameValue, &resource.Properties.Server)
	validator.AddRequiredInt32Field(renderers.Port, &resource.Properties.Port)
	validator.AddOptionalStringField(renderers.UsernameStringValue, &resource.Properties.Username)
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

func (p *Processor) computeConnectionString(resource *datamodel.SqlDatabase) string {
	var username, password string
	if resource.Properties.Username != "" {
		username = "User Id=" + resource.Properties.Username
	}
	if resource.Properties.Secrets.Password != "" {
		password = "Password=" + resource.Properties.Secrets.Password
	}

	connectionString := fmt.Sprintf("Data Source=tcp:%s,%v;Initial Catalog=%s;%s;%s;Encrypt=True;TrustServerCertificate=True", resource.Properties.Server, resource.Properties.Port, resource.Properties.Database, username, password)
	return connectionString
}
