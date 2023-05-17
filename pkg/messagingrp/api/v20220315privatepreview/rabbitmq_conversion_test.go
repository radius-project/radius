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

package v20220315privatepreview

import (
	"encoding/json"
	"os"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/messagingrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

type TestData struct {
	Description string          `json:"description,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

type fakeResource struct{}

func (f *fakeResource) ResourceTypeName() string {
	return "FakeResource"
}

func TestRabbitMQQueue_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"rabbitmqresource.json", "rabbitmqresource2.json", "rabbitmqresource_recipe.json"}
	for _, payload := range testset {

		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &RabbitMQQueueResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.RabbitMQQueue)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Messaging/rabbitMQQueues/rabbitmq0", convertedResource.ID)
		require.Equal(t, "rabbitmq0", convertedResource.Name)
		require.Equal(t, linkrp.N_RabbitMQQueuesResourceType, convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		switch versionedResource.Properties.(type) {
		case *ValuesRabbitMQQueueProperties:
			require.Equal(t, "values", string(convertedResource.Properties.Mode))
			require.Equal(t, "testQueue", string(convertedResource.Properties.Queue))
			require.Equal(t, "connection://string", convertedResource.Properties.Secrets.ConnectionString)
		case *RecipeRabbitMQQueueProperties:
			require.Equal(t, "recipe", string(convertedResource.Properties.Mode))
			require.Equal(t, "rabbitmq", convertedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", convertedResource.Properties.Recipe.Parameters["foo"])
			require.Equal(t, []rpv1.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}
	}
}

func TestRabbitMQQueue_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"rabbitmqresourcedatamodel.json", "rabbitmqresourcedatamodel2.json", "rabbitmqresourcedatamodel_recipe.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.RabbitMQQueue{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &RabbitMQQueueResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Messaging/rabbitMQQueues/rabbitmq0", *versionedResource.ID)
		require.Equal(t, "rabbitmq0", *versionedResource.Name)
		require.Equal(t, linkrp.N_RabbitMQQueuesResourceType, *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.GetRabbitMQQueueProperties().Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.GetRabbitMQQueueProperties().Environment)
		switch v := versionedResource.Properties.(type) {
		case *ValuesRabbitMQQueueProperties:
			require.Equal(t, "values", string(*v.Mode))
			require.Equal(t, "testQueue", *v.Queue)
		case *RecipeRabbitMQQueueProperties:
			require.Equal(t, "recipe", string(*v.Mode))
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "rabbitmqProvider", v.Status.OutputResources[0]["Provider"])
		}
	}
}

func TestRabbitMQQueue_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testsFile := "rabbitmqinvalid.json"
	rawPayload := loadTestData(testsFile)
	var testset []TestData
	err := json.Unmarshal(rawPayload, &testset)
	require.NoError(t, err)
	for _, testData := range testset {
		versionedResource := &RabbitMQQueueResource{}
		err := json.Unmarshal(testData.Payload, versionedResource)
		require.NoError(t, err)
		var expectedErr v1.ErrClientRP
		description := testData.Description
		if description == "unsupported_mode" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "Unsupported mode abc"
		}
		if description == "invalid_properties_with_mode_recipe" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "recipe is a required property for mode 'recipe'"
		}
		if description == "invalid_properties_with_mode_values" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "queue is a required property for mode 'values'"
		}
		_, err = versionedResource.ConvertTo()
		require.Equal(t, &expectedErr, err)
	}
}

func TestRabbitMQQueue_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &RabbitMQQueueResource{}
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
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &RabbitMQSecrets{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func loadTestData(testfile string) []byte {
	d, err := os.ReadFile("./testdata/" + testfile)
	if err != nil {
		return nil
	}
	return d
}
