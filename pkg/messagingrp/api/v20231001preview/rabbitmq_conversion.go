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

package v20231001preview

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/messagingrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
)

// ConvertTo converts a versioned RabbitMQQueueResource to a version-agnostic datamodel.RabbitMQQueue
// and returns it or an error if the inputs are invalid.
func (src *RabbitMQQueueResource) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.RabbitMQQueue{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       to.String(src.ID),
				Name:     to.String(src.Name),
				Type:     to.String(src.Type),
				Location: to.String(src.Location),
				Tags:     to.StringMap(src.Tags),
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: toProvisioningStateDataModel(src.Properties.ProvisioningState),
			},
		},
		Properties: datamodel.RabbitMQQueueProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: to.String(src.Properties.Environment),
				Application: to.String(src.Properties.Application),
			},
		},
	}
	properties := src.Properties
	var err error
	converted.Properties.ResourceProvisioning, err = toResourceProvisiongDataModel(properties.ResourceProvisioning)
	if err != nil {
		return nil, err
	}

	if converted.Properties.ResourceProvisioning != portableresources.ResourceProvisioningManual {
		converted.Properties.Recipe = toRecipeDataModel(properties.Recipe)
	}
	converted.Properties.Resources = toResourcesDataModel(properties.Resources)
	converted.Properties.Host = to.String(properties.Host)
	converted.Properties.Port = to.Int32(properties.Port)
	converted.Properties.Username = to.String(properties.Username)
	converted.Properties.Queue = to.String(properties.Queue)
	converted.Properties.VHost = to.String(properties.VHost)
	converted.Properties.TLS = to.Bool(properties.TLS)
	err = converted.VerifyInputs()
	if err != nil {
		return nil, err
	}

	if src.Properties.Secrets != nil {
		converted.Properties.Secrets = datamodel.RabbitMQSecrets{
			URI:      to.String(src.Properties.Secrets.URI),
			Password: to.String(properties.Secrets.Password),
		}
	}
	return converted, nil
}

// ConvertFrom converts a version-agnostic DataModelInterface to a versioned RabbitMQQueueResource,
// returning an error if the conversion fails.
func (dst *RabbitMQQueueResource) ConvertFrom(src v1.DataModelInterface) error {
	rabbitmq, ok := src.(*datamodel.RabbitMQQueue)
	if !ok {
		return v1.ErrInvalidModelConversion
	}

	dst.ID = new(rabbitmq.ID)
	dst.Name = new(rabbitmq.Name)
	dst.Type = new(rabbitmq.Type)
	dst.SystemData = fromSystemDataModel(rabbitmq.SystemData)
	dst.Location = new(rabbitmq.Location)
	dst.Tags = *to.StringMapPtr(rabbitmq.Tags)
	dst.Properties = &RabbitMQQueueProperties{
		Status: &ResourceStatus{
			OutputResources: toOutputResources(rabbitmq.Properties.Status.OutputResources),
			Recipe:          fromRecipeStatus(rabbitmq.Properties.Status.Recipe),
		},
		ProvisioningState:    fromProvisioningStateDataModel(rabbitmq.InternalMetadata.AsyncProvisioningState),
		Environment:          new(rabbitmq.Properties.Environment),
		Application:          new(rabbitmq.Properties.Application),
		ResourceProvisioning: fromResourceProvisioningDataModel(rabbitmq.Properties.ResourceProvisioning),
		Queue:                new(rabbitmq.Properties.Queue),
		Host:                 new(rabbitmq.Properties.Host),
		Port:                 new(rabbitmq.Properties.Port),
		VHost:                new(rabbitmq.Properties.VHost),
		Username:             new(rabbitmq.Properties.Username),
		Resources:            fromResourcesDataModel(rabbitmq.Properties.Resources),
		TLS:                  new(rabbitmq.Properties.TLS),
	}
	if rabbitmq.Properties.ResourceProvisioning == portableresources.ResourceProvisioningRecipe {
		dst.Properties.Recipe = fromRecipeDataModel(rabbitmq.Properties.Recipe)
	}

	return nil
}

// ConvertFrom converts a version-agnostic datamodel.RabbitMQSecrets to a versioned RabbitMQSecrets,
// returning an error if the conversion fails.
func (dst *RabbitMQSecrets) ConvertFrom(src v1.DataModelInterface) error {
	rabbitMQSecrets, ok := src.(*datamodel.RabbitMQSecrets)
	if !ok {
		return v1.ErrInvalidModelConversion
	}
	dst.URI = new(rabbitMQSecrets.URI)
	dst.Password = new(rabbitMQSecrets.Password)
	return nil
}

// ConvertTo converts a versioned RabbitMQSecrets object to a version-agnostic datamodel.RabbitMQSecrets object.
func (src *RabbitMQSecrets) ConvertTo() (v1.DataModelInterface, error) {
	converted := &datamodel.RabbitMQSecrets{
		URI:      to.String(src.URI),
		Password: to.String(src.Password),
	}
	return converted, nil
}
