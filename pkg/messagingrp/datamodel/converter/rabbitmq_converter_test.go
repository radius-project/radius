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

package converter

import (
	"encoding/json"
	"errors"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/messagingrp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/messagingrp/datamodel"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

// Validates type conversion between versioned client side data model and RP data model.
func TestRabbitMQQueueDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20231001preview/testdata/rabbitmq_manual_datamodel.json",
			"2023-10-01-preview",
			&v20231001preview.RabbitMQQueueResource{},
			nil,
		},
		{
			"../../api/v20231001preview/testdata/rabbitmq_manual_datamodel.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.dataModelFile)
			dm := &datamodel.RabbitMQQueue{}
			err := json.Unmarshal(c, dm)
			require.NoError(t, err)
			am, err := RabbitMQQueueDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}

func TestRabbitMQQueueDataModelFromVersioned(t *testing.T) {
	testset := []struct {
		versionedModelFile string
		apiVersion         string
		err                error
	}{
		{
			"../../api/v20231001preview/testdata/rabbitmq_manual_resource.json",
			"2023-10-01-preview",
			nil,
		},
		{
			"../../api/v20231001preview/testdata/rabbitmqresource-invalid.json",
			"2023-10-01-preview",
			errors.New("json: cannot unmarshal number into Go struct field RabbitMQQueueProperties.properties.resource of type string"),
		},
		{
			"../../api/v20231001preview/testdata/rabbitmq_manual_resource.json",
			"unsupported",
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.versionedModelFile)
			dm, err := RabbitMQQueueDataModelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiVersion, dm.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}

func TestRabbitMQSecretsDataModelToVersioned(t *testing.T) {
	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		err           error
	}{
		{
			"../../api/v20231001preview/testdata/rabbitmqsecretsdatamodel.json",
			"2023-10-01-preview",
			&v20231001preview.RabbitMQSecrets{},
			nil,
		},
		{
			"../../api/v20231001preview/testdata/rabbitmqsecretsdatamodel.json",
			"unsupported",
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.dataModelFile)
			dm := &datamodel.RabbitMQSecrets{}
			err := json.Unmarshal(c, dm)
			require.NoError(t, err)
			am, err := RabbitMQSecretsDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
			}
		})
	}
}
