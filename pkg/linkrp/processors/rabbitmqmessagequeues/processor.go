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

package rabbitmqmessagequeues

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

const (
	Queue = "queue"
	// RabbitMQSSLPort is the default port for RabbitMQ SSL connections.
	RabbitMQSSLPort = 5671
)

// Processor is a processor for RabbitMQQueue resources.
type Processor struct {
}

// Process implements the processors.Processor interface for RabbitMQQueue resources. It validates the required and computed
//  RabbitMQMessageQueue properties and applies the values from the RecipeOutput. It returns an error if validation fails.

func (p *Processor) Process(ctx context.Context, resource *datamodel.RabbitMQMessageQueue, options processors.Options) error {
	validator := processors.NewValidator(&resource.ComputedValues, &resource.SecretValues, &resource.Properties.Status.OutputResources)
	validator.AddResourcesField(&resource.Properties.Resources)
	validator.AddRequiredStringField(Queue, &resource.Properties.Queue)
	validator.AddRequiredStringField(renderers.Host, &resource.Properties.Host)
	validator.AddOptionalStringField(renderers.VHost, &resource.Properties.VHost)
	validator.AddRequiredInt32Field(renderers.Port, &resource.Properties.Port)
	validator.AddOptionalStringField(renderers.UsernameStringValue, &resource.Properties.Username)
	validator.AddOptionalSecretField(renderers.PasswordStringHolder, &resource.Properties.Secrets.Password)
	validator.AddComputedBoolField(renderers.TLS, &resource.Properties.TLS, func() (bool, *processors.ValidationError) {
		return p.computeSSL(resource), nil
	})
	validator.AddComputedSecretField(renderers.URI, &resource.Properties.Secrets.URI, func() (string, *processors.ValidationError) {
		return p.computeURI(resource), nil
	})

	err := validator.SetAndValidate(options.RecipeOutput)
	if err != nil {
		return err
	}

	return nil
}

func (p *Processor) computeURI(resource *datamodel.RabbitMQMessageQueue) string {
	rabbitMQProtocol := "amqp"
	if resource.Properties.TLS {
		rabbitMQProtocol = "amqps"
	}
	usernamePassword := ""
	if resource.Properties.Username != "" || resource.Properties.Secrets.Password != "" {
		usernamePassword = fmt.Sprintf("%s:%s@", resource.Properties.Username, resource.Properties.Secrets.Password)
	}
	return fmt.Sprintf("%s://%s%s:%v/%s", rabbitMQProtocol, usernamePassword, resource.Properties.Host, resource.Properties.Port, resource.Properties.VHost)
}

func (p *Processor) computeSSL(resource *datamodel.RabbitMQMessageQueue) bool {
	return resource.Properties.Port == RabbitMQSSLPort
}
