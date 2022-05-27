// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
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

		resourceType := map[string]interface{}{"Provider": "kubernetes", "Type": "DaprPubSubProvider"}
		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.DaprPubSubBroker)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprPubSubBrokers/daprPubSub0", convertedResource.ID)
		require.Equal(t, "daprPubSub0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/daprPubSubBrokers", convertedResource.Type)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)

		switch v := convertedResource.Properties.(type) {
		case *datamodel.DaprPubSubAzureServiceBusResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/testQueue", v.Resource)
			require.Equal(t, "pubsub.azure.servicebus", v.Kind)
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, v.Status.OutputResources[0]["ResourceType"])

		case *datamodel.DaprPubSubGenericResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "generic", v.Kind)
			require.Equal(t, "pubsub.kafka", v.Type)
			require.Equal(t, "v1", v.Version)
			require.Equal(t, "bar", v.Metadata["foo"])
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, v.Status.OutputResources[0]["ResourceType"])
		default:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.GetDaprPubSubBrokerProperties().Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.GetDaprPubSubBrokerProperties().Environment)
			require.Equal(t, "Deployment", convertedResource.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, convertedResource.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["ResourceType"])
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

		resourceType := map[string]interface{}{"Provider": "kubernetes", "Type": "DaprPubSubProvider"}
		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprPubSubBrokers/daprPubSub0", resource.ID)
		require.Equal(t, "daprPubSub0", resource.Name)
		require.Equal(t, "Applications.Connector/daprPubSubBrokers", resource.Type)
		switch v := resource.Properties.(type) {
		case *datamodel.DaprPubSubAzureServiceBusResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/testQueue", v.Resource)
			require.Equal(t, "pubsub.azure.servicebus", v.Kind)
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, v.Status.OutputResources[0]["ResourceType"])
		case *datamodel.DaprPubSubGenericResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "generic", v.Kind)
			require.Equal(t, "pubsub.kafka", v.Type)
			require.Equal(t, "v1", v.Version)
			require.Equal(t, "bar", v.Metadata["foo"])
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, v.Status.OutputResources[0]["ResourceType"])
		default:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.GetDaprPubSubBrokerProperties().Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.GetDaprPubSubBrokerProperties().Environment)
			require.Equal(t, "Deployment", resource.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, resource.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["ResourceType"])
		}
	}
}

func TestDaprPubSubBroker_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src api.DataModelInterface
		err error
	}{
		{&fakeResource{}, api.ErrInvalidModelConversion},
		{nil, api.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprPubSubBrokerResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
