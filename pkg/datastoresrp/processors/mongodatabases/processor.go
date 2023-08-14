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

package mongodatabases

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

// Processor is a processor for MongoDB resources.
type Processor struct {
}

// # Function Explanation
//
// Process implements the processors.Processor interface for Mongo database resources. It  validates Mongo database properties
// and applies the values from the RecipeOutput.
func (p *Processor) Process(ctx context.Context, resource *datamodel.MongoDatabase, options processors.Options) error {
	validator := processors.NewValidator(&resource.ComputedValues, &resource.SecretValues, &resource.Properties.Status.OutputResources)

	validator.AddResourcesField(&resource.Properties.Resources)
	validator.AddRequiredStringField(renderers.Host, &resource.Properties.Host)
	validator.AddRequiredInt32Field(renderers.Port, &resource.Properties.Port)
	validator.AddRequiredStringField(renderers.DatabaseNameValue, &resource.Properties.Database)
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

func (p *Processor) computeConnectionString(resource *datamodel.MongoDatabase) string {
	connectionString := "mongodb://"

	if resource.Properties.Username != "" {
		connectionString += resource.Properties.Username + ":"
	}
	if resource.Properties.Secrets.Password != "" {
		connectionString += resource.Properties.Secrets.Password + "@"
	}

	connectionString = fmt.Sprintf("%s%s:%v/%s", connectionString, resource.Properties.Host, resource.Properties.Port, resource.Properties.Database)
	return connectionString
}
