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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaprPubSubBroker_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"daprpubsubbrokerazureresource.json", "daprpubsubbrokergenericresource.json"}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &DaprPubSubBrokerResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.DaprPubSubBroker)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprPubSubBrokers/daprPubSub0", convertedResource.ID)
		require.Equal(t, "daprPubSub0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/daprPubSubBrokers", convertedResource.Type)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		switch convertedResource.Properties.Kind {
		case datamodel.DaprPubSubBrokerKindAzureServiceBus:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/testQueue", convertedResource.Properties.DaprPubSubAzureServiceBus.Resource)
			require.Equal(t, "pubsub.azure.servicebus", string(convertedResource.Properties.Kind))

		case datamodel.DaprPubSubBrokerKindGeneric:
			require.Equal(t, "generic", string(convertedResource.Properties.Kind))
			require.Equal(t, "pubsub.kafka", convertedResource.Properties.DaprPubSubGeneric.Type)
			require.Equal(t, "v1", convertedResource.Properties.DaprPubSubGeneric.Version)
			require.Equal(t, "bar", convertedResource.Properties.DaprPubSubGeneric.Metadata["foo"])
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		default:
			assert.Fail(t, "Kind of DaprPubSubBroker is specified.")
		}
	}

}

func TestDaprPubSubBroker_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"daprpubsubbrokerazureresourcedatamodel.json", "daprpubsubbrokergenericresourcedatamodel.json"}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.DaprPubSubBroker{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &DaprPubSubBrokerResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprPubSubBrokers/daprPubSub0", resource.ID)
		require.Equal(t, "daprPubSub0", resource.Name)
		require.Equal(t, "Applications.Connector/daprPubSubBrokers", resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
		switch resource.Properties.Kind {
		case datamodel.DaprPubSubBrokerKindAzureServiceBus:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/testQueue", resource.Properties.DaprPubSubAzureServiceBus.Resource)
			require.Equal(t, "pubsub.azure.servicebus", string(resource.Properties.Kind))
			require.Equal(t, "Deployment", versionedResource.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["LocalID"])
			require.Equal(t, "kubernetes", versionedResource.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["Provider"])
		case datamodel.DaprPubSubBrokerKindGeneric:
			require.Equal(t, "generic", string(resource.Properties.Kind))
			require.Equal(t, "pubsub.kafka", resource.Properties.DaprPubSubGeneric.Type)
			require.Equal(t, "v1", resource.Properties.DaprPubSubGeneric.Version)
			require.Equal(t, "bar", resource.Properties.DaprPubSubGeneric.Metadata["foo"])
		default:
			assert.Fail(t, "Kind of DaprPubSubBroker is specified.")
		}
	}
}

func TestDaprPubSubBroker_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprPubSubBrokerResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
