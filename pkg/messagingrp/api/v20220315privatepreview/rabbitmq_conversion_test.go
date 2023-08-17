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
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/messagingrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

type fakeResource struct{}

// ResourceTypeName returns type of the resource.
func (f *fakeResource) ResourceTypeName() string {
	return "FakeResource"
}

func TestRabbitMQQueue_ConvertVersionedToDataModel(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *datamodel.RabbitMQQueue
	}{
		{
			desc: "rabbitmq manual resource",
			file: "rabbitmq_manual_resource.json",
			expected: &datamodel.RabbitMQQueue{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Messaging/rabbitMQQueues/rabbitmq0",
						Name:     "rabbitmq0",
						Type:     linkrp.N_RabbitMQQueuesResourceType,
						Location: v1.LocationGlobal,
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.RabbitMQQueueProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: linkrp.ResourceProvisioningManual,
					Queue:                "testQueue",
					Host:                 "test-host",
					VHost:                "test-vhost",
					Port:                 5672,
					Username:             "test-user",
					TLS:                  true,
					Secrets: datamodel.RabbitMQSecrets{
						URI:      "connection://string",
						Password: "password",
					},
				},
			},
		},
		{
			desc: "rabbitmq recipe resource",
			file: "rabbitmq_recipe_resource.json",
			expected: &datamodel.RabbitMQQueue{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Messaging/rabbitMQQueues/rabbitmq0",
						Name:     "rabbitmq0",
						Type:     linkrp.N_RabbitMQQueuesResourceType,
						Location: v1.LocationGlobal,
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.RabbitMQQueueProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: linkrp.ResourceProvisioningRecipe,
					TLS:                  false,
					Recipe: linkrp.LinkRecipe{
						Name: "rabbitmq",
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// arrange
			rawPayload, err := loadTestData("./testdata/" + tc.file)
			require.NoError(t, err)
			versionedResource := &RabbitMQQueueResource{}
			err = json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			// act
			dm, err := versionedResource.ConvertTo()

			// assert
			require.NoError(t, err)
			convertedResource := dm.(*datamodel.RabbitMQQueue)

			require.Equal(t, tc.expected, convertedResource)
		})
	}
}

func TestRabbitMQQueue_ConvertDataModelToVersioned(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *RabbitMQQueueResource
	}{
		{
			desc: "rabbitmq manual data model",
			file: "rabbitmq_manual_datamodel.json",
			expected: &RabbitMQQueueResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &RabbitMQQueueProperties{
					Environment:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					ResourceProvisioning: to.Ptr(ResourceProvisioningManual),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Queue:                to.Ptr("testQueue"),
					Host:                 to.Ptr("test-host"),
					VHost:                to.Ptr("test-vhost"),
					Port:                 to.Ptr(int32(5672)),
					Username:             to.Ptr("test-user"),
					TLS:                  to.Ptr(true),
					Status: &ResourceStatus{
						OutputResources: []map[string]any{
							{
								"Identity": nil,
								"LocalID":  "Deployment",
								"Provider": "rabbitmqProvider",
							},
						},
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Messaging/rabbitMQQueues/rabbitmq0"),
				Name: to.Ptr("rabbitmq0"),
				Type: to.Ptr(linkrp.N_RabbitMQQueuesResourceType),
			},
		},
		{
			desc: "rabbitmq recipe data model",
			file: "rabbitmq_recipe_datamodel.json",
			expected: &RabbitMQQueueResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &RabbitMQQueueProperties{
					Environment:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					ResourceProvisioning: to.Ptr(ResourceProvisioningRecipe),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Queue:                to.Ptr("testQueue"),
					Host:                 to.Ptr("test-host"),
					VHost:                to.Ptr("test-vhost"),
					Port:                 to.Ptr(int32(5672)),
					Username:             to.Ptr("test-user"),
					TLS:                  to.Ptr(false),
					Recipe: &Recipe{
						Name: to.Ptr("rabbitmq"),
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
					Status: &ResourceStatus{
						OutputResources: []map[string]any{
							{
								"Identity": nil,
								"LocalID":  "Deployment",
								"Provider": "rabbitmqProvider",
							},
						},
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Messaging/rabbitMQQueues/rabbitmq0"),
				Name: to.Ptr("rabbitmq0"),
				Type: to.Ptr(linkrp.N_RabbitMQQueuesResourceType),
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			rawPayload, err := loadTestData("./testdata/" + tc.file)
			require.NoError(t, err)
			resource := &datamodel.RabbitMQQueue{}
			err = json.Unmarshal(rawPayload, resource)
			require.NoError(t, err)

			versionedResource := &RabbitMQQueueResource{}
			err = versionedResource.ConvertFrom(resource)
			require.NoError(t, err)

			// Skip system data comparison
			versionedResource.SystemData = nil

			require.Equal(t, tc.expected, versionedResource)
		})
	}
}

func TestRabbitMQQueue_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testset := []struct {
		payload string
		errType error
		message string
	}{
		{
			"rabbitmq_invalid_properties_resource.json",
			&v1.ErrClientRP{},
			"code Bad Request: err queue is required when resourceProvisioning is manual",
		},
		{
			"rabbitmq_invalid_resourceprovisioning_resource.json",
			&v1.ErrModelConversion{},
			"$.properties.resourceProvisioning must be one of [manual recipe].",
		},
	}

	for _, test := range testset {
		t.Run(test.payload, func(t *testing.T) {
			rawPayload, err := loadTestData("./testdata/" + test.payload)
			require.NoError(t, err)
			versionedResource := &RabbitMQQueueResource{}
			err = json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			dm, err := versionedResource.ConvertTo()
			require.Error(t, err)
			require.Nil(t, dm)
			require.IsType(t, test.errType, err)
			require.Equal(t, test.message, err.Error())
		})
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
	rawPayload, err := loadTestData("./testdata/rabbitmqsecrets.json")
	require.NoError(t, err)
	versioned := &RabbitMQSecrets{}
	err = json.Unmarshal(rawPayload, versioned)
	require.NoError(t, err)

	// act
	dm, err := versioned.ConvertTo()

	// assert
	require.NoError(t, err)
	converted := dm.(*datamodel.RabbitMQSecrets)
	require.Equal(t, "test-connection-string", converted.URI)
}

func TestRabbitMQSecrets_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload, err := loadTestData("./testdata/rabbitmqsecretsdatamodel.json")
	require.NoError(t, err)
	secrets := &datamodel.RabbitMQSecrets{}
	err = json.Unmarshal(rawPayload, secrets)
	require.NoError(t, err)

	// act
	versionedResource := &RabbitMQSecrets{}
	err = versionedResource.ConvertFrom(secrets)

	// assert
	require.NoError(t, err)
	require.Equal(t, "test-connection-string", secrets.URI)
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
