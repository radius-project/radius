// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converter

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
)

// RabbitMQMessageQueueDataModelFromVersioned converts version agnostic RabbitMQMessageQueue datamodel to versioned model.
func RabbitMQMessageQueueDataModelToVersioned(model *datamodel.RabbitMQMessageQueue, version string) (conv.VersionedModelInterface, error) {
	switch version {
	case v20220315privatepreview.Version:
		versioned := &v20220315privatepreview.RabbitMQMessageQueueResource{}
		err := versioned.ConvertFrom(model)
		return versioned, err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}

// RabbitMQMessageQueueDataModelToVersioned converts versioned RabbitMQMessageQueue model to datamodel.
func RabbitMQMessageQueueDataModelFromVersioned(content []byte, version string) (*datamodel.RabbitMQMessageQueue, error) {
	switch version {
	case v20220315privatepreview.Version:
		am := &v20220315privatepreview.RabbitMQMessageQueueResource{}
		if err := json.Unmarshal(content, am); err != nil {
			return nil, err
		}
		dm, err := am.ConvertTo()
		return dm.(*datamodel.RabbitMQMessageQueue), err

	default:
		return nil, v1.ErrUnsupportedAPIVersion
	}
}
