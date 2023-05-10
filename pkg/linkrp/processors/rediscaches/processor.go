// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

const (
	// RedisNonSSLPort is the default port for Redis non-SSL connections.
	RedisNonSSLPort = 6379

	// RedisSSLPort is the default port for Redis SSL connections.
	RedisSSLPort = 6380
)

// Processor is a processor for RedisCache resources.
type Processor struct {
}

// Process implements the processors.Processor interface for RedisCache resources.
func (p *Processor) Process(ctx context.Context, resource *datamodel.RedisCache, options processors.Options) error {
	validator := processors.NewValidator(&resource.ComputedValues, &resource.SecretValues, &resource.Properties.Status.OutputResources)

	validator.AddResourcesField(&resource.Properties.Resources)
	validator.AddRequiredStringField(renderers.Host, &resource.Properties.Host)
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

func (p *Processor) computeConnectionString(resource *datamodel.RedisCache) string {
	ssl := resource.Properties.Port == RedisSSLPort
	connectionString := fmt.Sprintf("%s:%v,abortConnect=False", resource.Properties.Host, resource.Properties.Port)
	if ssl {
		connectionString = connectionString + ",ssl=True"
	}

	if resource.Properties.Username != "" {
		connectionString = connectionString + ",user=" + resource.Properties.Username
	}
	if resource.Properties.Secrets.Password != "" {
		connectionString = connectionString + ",password=" + resource.Properties.Secrets.Password
	}

	return connectionString
}
