// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaprPubSubBroker_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{
		"daprpubsubbrokerazureresource.json",
		"daprpubsubbrokerazureresource_recipe.json",
		"daprpubsubbrokerazureresource_recipe2.json",
		"daprpubsubbrokergenericresource.json",
		"daprpubsubbrokergenericresource_recipe.json",
		"daprpubsubbrokergenericresource_recipe2.json"}

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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/daprPubSubBrokers/daprPubSub0", convertedResource.ID)
		require.Equal(t, "daprPubSub0", convertedResource.Name)
		require.Equal(t, "Applications.Link/daprPubSubBrokers", convertedResource.Type)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		if convertedResource.Properties.Mode != datamodel.DaprPubSubBrokerModeRecipe {
			switch convertedResource.Properties.Kind {
			case datamodel.DaprPubSubBrokerKindAzureServiceBus:
				if convertedResource.Properties.Mode == datamodel.DaprPubSubBrokerModeResource {
					require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/testQueue", convertedResource.Properties.Resource)
					require.Equal(t, "pubsub.azure.servicebus", string(convertedResource.Properties.Kind))
				}

			case datamodel.DaprPubSubBrokerKindGeneric:
				if convertedResource.Properties.Mode == datamodel.DaprPubSubBrokerModeValues {
					require.Equal(t, "generic", string(convertedResource.Properties.Kind))
					require.Equal(t, "pubsub.kafka", convertedResource.Properties.Type)
					require.Equal(t, "v1", convertedResource.Properties.Version)
					require.Equal(t, "bar", convertedResource.Properties.Metadata["foo"])
					require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
				}
			default:
				assert.Fail(t, "Kind of DaprPubSubBroker is specified.")
			}
		}

		if payload == "daprpubsubbrokerazureresource_recipe.json" ||
			payload == "daprpubsubbrokerazureresource_recipe2.json" ||
			payload == "daprpubsubbrokergenericresource_recipe.json" ||
			payload == "daprpubsubbrokergenericresource_recipe2.json" {
			require.Equal(t, "redis-test", convertedResource.Properties.Recipe.Name)
			if payload == "daprpubsubbrokerazureresource_recipe2.json" || payload == "daprpubsubbrokergenericresource_recipe2.json" {
				parameters := map[string]interface{}{"port": float64(6081)}
				require.Equal(t, parameters, convertedResource.Properties.Recipe.Parameters)
			}
		}
	}

}

func TestDaprPubSubBroker_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{
		"daprpubsubbrokerazureresourcedatamodel.json",
		"daprpubsubbrokerazureresourcedatamodel_recipe.json",
		"daprpubsubbrokerazureresourcedatamodel_recipe2.json",
		"daprpubsubbrokergenericresourcedatamodel.json",
		"daprpubsubbrokergenericresourcedatamodel_recipe.json",
		"daprpubsubbrokergenericresourcedatamodel_recipe2.json"}

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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/daprPubSubBrokers/daprPubSub0", resource.ID)
		require.Equal(t, "daprPubSub0", resource.Name)
		require.Equal(t, "Applications.Link/daprPubSubBrokers", resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
		if resource.Properties.Mode != datamodel.DaprPubSubBrokerModeRecipe {
			switch resource.Properties.Kind {
			case datamodel.DaprPubSubBrokerKindAzureServiceBus:
				if resource.Properties.Mode == datamodel.DaprPubSubBrokerModeResource {
					require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/testQueue", resource.Properties.Resource)
					require.Equal(t, "pubsub.azure.servicebus", string(resource.Properties.Kind))
					require.Equal(t, "Deployment", versionedResource.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["LocalID"])
					require.Equal(t, "kubernetes", versionedResource.Properties.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["Provider"])
				}
			case datamodel.DaprPubSubBrokerKindGeneric:
				if resource.Properties.Mode == datamodel.DaprPubSubBrokerModeValues {
					require.Equal(t, "generic", string(resource.Properties.Kind))
					require.Equal(t, "pubsub.kafka", resource.Properties.Type)
					require.Equal(t, "v1", resource.Properties.Version)
					require.Equal(t, "bar", resource.Properties.Metadata["foo"])
				}
			default:
				assert.Fail(t, "Kind of DaprPubSubBroker is specified.")
			}
		}

		if payload == "daprpubsubbrokerazureresourcedatamodel_recipe.json" ||
			payload == "daprpubsubbrokerazureresourcedatamodel_recipe2.json" ||
			payload == "daprpubsubbrokergenericresourcedatamodel_recipe.json" ||
			payload == "daprpubsubbrokergenericresourcedatamodel_recipe2.json" {
			require.Equal(t, "redis-test", resource.Properties.Recipe.Name)
			if payload == "daprpubsubbrokerazureresourcedatamodel_recipe2.json" || payload == "daprpubsubbrokergenericresourcedatamodel_recipe2.json" {
				parameters := map[string]interface{}{"port": float64(6081)}
				require.Equal(t, parameters, resource.Properties.Recipe.Parameters)
			}
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
