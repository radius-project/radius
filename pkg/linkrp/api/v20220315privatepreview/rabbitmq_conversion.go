// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp"

	"github.com/Azure/go-autorest/autorest/to"
)

// ConvertTo converts from the versioned RabbitMQMessageQueue resource to version-agnostic datamodel.
func (src *RabbitMQMessageQueueResource) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.RabbitMQMessageQueue{
		TrackedResource: v1.TrackedResource{
			ID:       to.String(src.ID),
			Name:     to.String(src.Name),
			Type:     to.String(src.Type),
			Location: to.String(src.Location),
			Tags:     to.StringMap(src.Tags),
		},
		Properties: datamodel.RabbitMQMessageQueueProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: to.String(src.Properties.GetRabbitMQMessageQueueProperties().Environment),
				Application: to.String(src.Properties.GetRabbitMQMessageQueueProperties().Application),
			},
			ProvisioningState: toProvisioningStateDataModel(src.Properties.GetRabbitMQMessageQueueProperties().ProvisioningState),
		},
		InternalMetadata: v1.InternalMetadata{
			UpdatedAPIVersion: Version,
		},
	}
	switch v := src.Properties.(type) {
	case *ValuesRabbitMQMessageQueueProperties:
		if v.Queue == nil {
			return nil, conv.NewClientErrInvalidRequest("queue is a required property for mode 'values'")
		}
		converted.Properties.Queue = to.String(v.Queue)
		converted.Properties.Mode = datamodel.RabbitMQMessageQueuePropertiesModeValues
	case *RecipeRabbitMQMessageQueueProperties:
		if v.Recipe == nil {
			return nil, conv.NewClientErrInvalidRequest("recipe is a required property for mode 'recipe'")
		}
		converted.Properties.Recipe = toRecipeDataModel(v.Recipe)
		converted.Properties.Queue = to.String(v.Queue)
		converted.Properties.Mode = datamodel.RabbitMQMessageQueuePropertiesModeRecipe
	default:
		return nil, conv.NewClientErrInvalidRequest("Invalid Mode for rabbitmq message queue")
	}
	if src.Properties.GetRabbitMQMessageQueueProperties().Secrets != nil {
		converted.Properties.Secrets = datamodel.RabbitMQSecrets{
			ConnectionString: to.String(src.Properties.GetRabbitMQMessageQueueProperties().Secrets.ConnectionString),
		}
	}
	return converted, nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RabbitMQMessageQueue resource.
func (dst *RabbitMQMessageQueueResource) ConvertFrom(src conv.DataModelInterface) error {
	rabbitmq, ok := src.(*datamodel.RabbitMQMessageQueue)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ID = to.StringPtr(rabbitmq.ID)
	dst.Name = to.StringPtr(rabbitmq.Name)
	dst.Type = to.StringPtr(rabbitmq.Type)
	dst.SystemData = fromSystemDataModel(rabbitmq.SystemData)
	dst.Location = to.StringPtr(rabbitmq.Location)
	dst.Tags = *to.StringMapPtr(rabbitmq.Tags)
	switch rabbitmq.Properties.Mode {
	case datamodel.RabbitMQMessageQueuePropertiesModeValues:
		mode := RabbitMQMessageQueuePropertiesModeValues
		dst.Properties = &ValuesRabbitMQMessageQueueProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(rabbitmq.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(rabbitmq.Properties.ProvisioningState),
			Environment:       to.StringPtr(rabbitmq.Properties.Environment),
			Application:       to.StringPtr(rabbitmq.Properties.Application),
			Mode:              &mode,
			Queue:             to.StringPtr(rabbitmq.Properties.Queue),
		}
	case datamodel.RabbitMQMessageQueuePropertiesModeRecipe:
		mode := RabbitMQMessageQueuePropertiesModeRecipe
		var recipe *Recipe
		if rabbitmq.Properties.Recipe.Name != "" {
			recipe = fromRecipeDataModel(rabbitmq.Properties.Recipe)
		}
		dst.Properties = &RecipeRabbitMQMessageQueueProperties{
			Status: &ResourceStatus{
				OutputResources: rp.BuildExternalOutputResources(rabbitmq.Properties.Status.OutputResources),
			},
			ProvisioningState: fromProvisioningStateDataModel(rabbitmq.Properties.ProvisioningState),
			Environment:       to.StringPtr(rabbitmq.Properties.Environment),
			Application:       to.StringPtr(rabbitmq.Properties.Application),
			Mode:              &mode,
			Queue:             to.StringPtr(rabbitmq.Properties.Queue),
			Recipe:            recipe,
		}
	default:
		return fmt.Errorf("unsupported mode %s", rabbitmq.Properties.Mode)
	}
	return nil
}

// ConvertFrom converts from version-agnostic datamodel to the versioned RabbitMQSecrets instance.
func (dst *RabbitMQSecrets) ConvertFrom(src conv.DataModelInterface) error {
	rabbitMQSecrets, ok := src.(*datamodel.RabbitMQSecrets)
	if !ok {
		return conv.ErrInvalidModelConversion
	}

	dst.ConnectionString = to.StringPtr(rabbitMQSecrets.ConnectionString)
	return nil
}

// ConvertTo converts from the versioned RabbitMQSecrets instance to version-agnostic datamodel.
func (src *RabbitMQSecrets) ConvertTo() (conv.DataModelInterface, error) {
	converted := &datamodel.RabbitMQSecrets{
		ConnectionString: to.String(src.ConnectionString),
	}
	return converted, nil
}
