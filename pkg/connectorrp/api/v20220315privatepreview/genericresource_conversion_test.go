// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestGenericResource_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"genericresource.json"}
	for _, payload := range testset {

		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &GenericResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.GenericResourceVersionAgnostic)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/rabbitMQMessageQueues/rabbitmq0", convertedResource.ID)
		require.Equal(t, "rabbitmq0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/rabbitMQMessageQueues", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.ResourceProperties["application"])
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.ResourceProperties["environment"])
		require.Equal(t, "testQueue", convertedResource.ResourceProperties["queue"])
	}
}

func TestGenericResource_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"genericresource.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.GenericResourceVersionAgnostic{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &GenericResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/rabbitMQMessageQueues/rabbitmq0", resource.ID)
		require.Equal(t, "rabbitmq0", resource.Name)
		require.Equal(t, "Applications.Connector/rabbitMQMessageQueues", resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.ResourceProperties["application"])
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.ResourceProperties["environment"])
		require.Equal(t, "testQueue", resource.ResourceProperties["queue"])
	}
}
