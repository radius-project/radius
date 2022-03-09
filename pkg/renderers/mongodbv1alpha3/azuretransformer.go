// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/project-radius/radius/pkg/renderers"
)

var _ renderers.SecretValueTransformer = (*AzureTransformer)(nil)

type AzureTransformer struct {
}

func (t *AzureTransformer) Transform(ctx context.Context, dependency renderers.RendererDependency, value interface{}) (interface{}, error) {
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

	databaseName, ok := dependency.ComputedValues[renderers.DatabaseValue].(string)
	if !ok {
		return nil, errors.New("expected the databaseName to be a string")
	}

	u.Path = "/" + databaseName
	return u.String(), nil
}
