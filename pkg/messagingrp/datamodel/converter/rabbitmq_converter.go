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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/messagingrp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/messagingrp/datamodel"
)

// RabbitMQQueueDataModelToVersioned converts a version-agnostic datamodel.RabbitMQQueue to a versioned model interface
// and returns an error if the version is unsupported.
func RabbitMQQueueDataModelToVersioned(model *datamodel.RabbitMQQueue, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.RabbitMQQueueResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err
	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// RabbitMQQueueDataModelFromVersioned takes in a byte slice and a version string and returns a version-agnostic
// RabbitMQQueue datamodel and an error if the version is unsupported.
func RabbitMQQueueDataModelFromVersioned(content []byte, version string) (*datamodel.RabbitMQQueue, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.RabbitMQQueueResource{}
		if err := json.Unmarshal(content, versioned); err != nil {
			return nil, err
		}
		dm, err := versioned.ConvertTo()
		return dm.(*datamodel.RabbitMQQueue), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// RabbitMQSecretsDataModelToVersioned converts a version-agnostic datamodel.RabbitMQSecrets to a versioned model
// based on the given version string, or returns an error if the version is not supported.
func RabbitMQSecretsDataModelToVersioned(model *datamodel.RabbitMQSecrets, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20231001preview.Version:
		versioned := &v20231001preview.RabbitMQSecrets{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
