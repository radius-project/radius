// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestRabbitMQMessageQueue_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("rabbitmqresource.json")
	versionedResource := &RabbitMQMessageQueueResource{}
	err := json.Unmarshal(rawPayload, versionedResource)
	require.NoError(t, err)

	// act
	dm, err := versionedResource.ConvertTo()

	// assert
	require.NoError(t, err)
	convertedResource := dm.(*datamodel.RabbitMQMessageQueue)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/rabbitMQMessageQueues/rabbitmq0", convertedResource.ID)
	require.Equal(t, "rabbitmq0", convertedResource.Name)
	require.Equal(t, "Applications.Connector/rabbitMQMessageQueues", convertedResource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
	require.Equal(t, "testQueue", convertedResource.Properties.Queue)
	require.Equal(t, "connection://string", convertedResource.Properties.Secrets.ConnectionString)
}

func TestRabbitMQMessageQueue_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("rabbitmqresourcedatamodel.json")
	resource := &datamodel.RabbitMQMessageQueue{}
	err := json.Unmarshal(rawPayload, resource)
	require.NoError(t, err)

	// act
	versionedResource := &RabbitMQMessageQueueResource{}
	err = versionedResource.ConvertFrom(resource)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/rabbitMQMessageQueues/rabbitmq0", resource.ID)
	require.Equal(t, "rabbitmq0", resource.Name)
	require.Equal(t, "Applications.Connector/rabbitMQMessageQueues", resource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
	require.Equal(t, "testQueue", resource.Properties.Queue)
	require.Equal(t, "connection://string", resource.Properties.Secrets.ConnectionString)
}
func TestRabbitMQMessageQueue_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &RabbitMQMessageQueueResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func TestRabbitMQSecrets_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("rabbitmqsecrets.json")
	versioned := &RabbitMQSecrets{}
	err := json.Unmarshal(rawPayload, versioned)
	require.NoError(t, err)

	// act
	dm, err := versioned.ConvertTo()

	// assert
	require.NoError(t, err)
	converted := dm.(*datamodel.RabbitMQSecrets)
	require.Equal(t, "test-connection-string", converted.ConnectionString)
}

func TestRabbitMQSecrets_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("rabbitmqsecretsdatamodel.json")
	secrets := &datamodel.RabbitMQSecrets{}
	err := json.Unmarshal(rawPayload, secrets)
	require.NoError(t, err)

	// act
	versionedResource := &RabbitMQSecrets{}
	err = versionedResource.ConvertFrom(secrets)

	// assert
	require.NoError(t, err)
	require.Equal(t, "test-connection-string", secrets.ConnectionString)
}

func TestRabbitMQSecrets_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &RabbitMQSecrets{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
