// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
)

var _ renderers.SecretValueTransformer = (*AzureTransformer)(nil)

type AzureTransformer struct {
}

func (t *AzureTransformer) Transform(ctx context.Context, resource conv.DataModelInterface, value interface{}) (interface{}, error) {
	mongoResource, ok := resource.(*datamodel.MongoDatabase)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	// Mongo uses the following format for mongo: mongodb://{accountname}:{key}@{endpoint}:{port}/{database}?...{params}
	//
	// The connection strings that come back from CosmosDB don't include the database name.
	str, ok := value.(string)
	if !ok {
		return nil, errors.New("expected the connection string to be a string")
	}

	// These connection strings won't include the database
	u, err := url.Parse(str)
	if err != nil {
		return "", fmt.Errorf("failed to parse connection string as a URL: %w", err)
	}

	databaseName, ok := mongoResource.InternalMetadata.ComputedValues[renderers.DatabaseNameValue].(string)
	if !ok {
		return nil, errors.New("expected the databaseName to be a string")
	}

	u.Path = "/" + databaseName
	return u.String(), nil
}
