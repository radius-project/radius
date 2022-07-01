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
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/stretchr/testify/require"
)

func TestRabbitMQMessageQueue_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"rabbitmqresource.json", "rabbitmqresource2.json"}
	for _, payload := range testset {

		// arrange
		rawPayload := loadTestData(payload)
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
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		if payload == "rabbitmqresource.json" {
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}
	}
}

func TestRabbitMQMessageQueue_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"rabbitmqresourcedatamodel.json", "rabbitmqresourcedatamodel2.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.RabbitMQMessageQueue{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &RabbitMQMessageQueueResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/rabbitMQMessageQueues/rabbitmq0", *versionedResource.ID)
		require.Equal(t, "rabbitmq0", *versionedResource.Name)
		require.Equal(t, "Applications.Connector/rabbitMQMessageQueues", *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		require.Equal(t, "testQueue", *versionedResource.Properties.Queue)
		require.Equal(t, "connection://string", *versionedResource.Properties.Secrets.ConnectionString)
		if payload == "rabbitmqresourcedatamodel.json" {
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "rabbitmqProvider", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}
	}
}

func TestRabbitMQMessageQueueResponse_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"rabbitmqresource.json", "rabbitmqresource2.json"}
	for _, payload := range testset {

		// arrange
		rawPayload := loadTestData(payload)
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
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		if payload == "rabbitmqresource.json" {
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}
	}
}

func TestRabbitMQMessageQueueResponse_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"rabbitmqresponseresourcedatamodel.json", "rabbitmqresourcedatamodel2.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.RabbitMQMessageQueue{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &RabbitMQMessageQueueResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/rabbitMQMessageQueues/rabbitmq0", *versionedResource.ID)
		require.Equal(t, "rabbitmq0", *versionedResource.Name)
		require.Equal(t, "Applications.Connector/rabbitMQMessageQueues", *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		require.Equal(t, "testQueue", *versionedResource.Properties.Queue)
		if payload == "rabbitmqresponseresourcedatamodel.json" {
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "rabbitmqProvider", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}
	}
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
