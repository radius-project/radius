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

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
)

const (
	Queue = "queue"
)

// Processor is a processor for RabbitMQQueue resources.
type Processor struct {
}

// # Function Explanation
//
// Process implements the processors.Processor interface for RabbitMQQueue resources. It validates the required and computed
//  RabbitMQMessageQueue properties and applies the values from the RecipeOutput. It returns an error if validation fails.

func (p *Processor) Process(ctx context.Context, resource *datamodel.RabbitMQMessageQueue, options processors.Options) error {
	validator := processors.NewValidator(&resource.ComputedValues, &resource.SecretValues, &resource.Properties.Status.OutputResources)
	validator.AddRequiredStringField(Queue, &resource.Properties.Queue)

	validator.AddComputedSecretField(renderers.ConnectionStringValue, &resource.Properties.Secrets.ConnectionString, func() (string, *processors.ValidationError) {
		return p.computeConnectionString(resource), nil
	})

	err := validator.SetAndValidate(options.RecipeOutput)
	if err != nil {
		return err
	}

	return nil
}

func (p *Processor) computeConnectionString(resource *datamodel.RabbitMQMessageQueue) string {
	return resource.Properties.Secrets.ConnectionString
}
