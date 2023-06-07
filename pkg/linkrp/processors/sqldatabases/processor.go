// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

// Processor is a processor for SqlDatabase resources.
type Processor struct {
}

// Process implements the processors.Processor interface for SqlDatabase resources.
func (p *Processor) Process(ctx context.Context, resource *datamodel.SqlDatabase, options processors.Options) error {
	validator := processors.NewValidator(&resource.ComputedValues, &resource.SecretValues, &resource.Properties.Status.OutputResources)

	validator.AddResourcesField(&resource.Properties.Resources)
	validator.AddRequiredStringField(renderers.DatabaseNameValue, &resource.Properties.Database)
	validator.AddRequiredStringField(renderers.ServerNameValue, &resource.Properties.Server)

	err := validator.SetAndValidate(options.RecipeOutput)
	if err != nil {
		return err
	}

	return nil
}
