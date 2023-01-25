// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

func TestDaprPubSubBroker_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{
		"daprpubsubbrokerazureresource.json",
		"daprpubsubbrokerresource_recipe.json",
		"daprpubsubbrokerresource_recipe2.json",
		"daprpubsubbrokergenericresource.json"}

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
		require.Equal(t, linkrp.DaprPubSubBrokersResourceType, convertedResource.Type)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		switch versionedResource.Properties.(type) {
		case *ResourceDaprPubSubProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/testQueue", convertedResource.Properties.Resource)
		case *ValuesDaprPubSubProperties:
			require.Equal(t, "pubsub.kafka", convertedResource.Properties.Type)
			require.Equal(t, "v1", convertedResource.Properties.Version)
			require.Equal(t, "bar", convertedResource.Properties.Metadata["foo"])
			require.Equal(t, []rpv1.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		case *RecipeDaprPubSubProperties:
			if payload == "daprpubsubbrokerresource_recipe2.json" {
				parameters := map[string]any{"port": float64(6081)}
				require.Equal(t, parameters, convertedResource.Properties.Recipe.Parameters)
			} else {
				require.Equal(t, "redis-test", convertedResource.Properties.Recipe.Name)
			}
		}
	}

}

func TestDaprPubSubBroker_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{
		"daprpubsubbrokerazureresourcedatamodel.json",
		"daprpubsubbrokergenericresourcedatamodel.json",
		"daprpubsubbrokerresourcedatamodel_recipe.json",
		"daprpubsubbrokerresourcedatamodel_recipe2.json"}

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
		require.Equal(t, linkrp.DaprPubSubBrokersResourceType, resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
		switch v := versionedResource.Properties.(type) {
		case *ResourceDaprPubSubProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/testQueue", *v.Resource)
			require.Equal(t, "Deployment", v.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["LocalID"])
			require.Equal(t, "kubernetes", v.GetDaprPubSubBrokerProperties().Status.OutputResources[0]["Provider"])
		case *ValuesDaprPubSubProperties:
			require.Equal(t, "pubsub.kafka", *v.Type)
			require.Equal(t, "v1", *v.Version)
			require.Equal(t, "bar", v.Metadata["foo"])
		case *RecipeDaprPubSubProperties:
			if payload == "daprpubsubbrokerresourcedatamodel_recipe2" {
				parameters := map[string]any{"port": float64(6081)}
				require.Equal(t, parameters, resource.Properties.Recipe.Parameters)
			} else {
				require.Equal(t, "redis-test", resource.Properties.Recipe.Name)
			}
		}
	}
}

func TestDaprPubSubBroker_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprPubSubBrokerResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
