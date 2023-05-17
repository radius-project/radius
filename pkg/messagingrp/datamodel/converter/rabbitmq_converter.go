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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/messagingrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/messagingrp/datamodel"
)

// RabbitMQQueueDataModelFromVersioned converts version agnostic RabbitMQQueue datamodel to versioned model.
func RabbitMQQueueDataModelToVersioned(model *datamodel.RabbitMQQueue, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.RabbitMQQueueResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err
	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// RabbitMQQueueDataModelFromVersioned converts versioned RabbitMQQueue model to datamodel.
func RabbitMQQueueDataModelFromVersioned(content []byte, version string) (*datamodel.RabbitMQQueue, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.RabbitMQQueueResource{}
		if err := json.Unmarshal(content, versioned); err != nil {
			return nil, err
		}
		dm, err := versioned.ConvertTo()
		return dm.(*datamodel.RabbitMQQueue), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// RabbitMQSecretsDataModelFromVersioned converts version agnostic RabbitMQSecrets datamodel to versioned model.
func RabbitMQSecretsDataModelToVersioned(model *datamodel.RabbitMQSecrets, version string) (v1.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.RabbitMQSecrets{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
