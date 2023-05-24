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

package mongodatabases

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/project-radius/radius/pkg/linkrp/renderers"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
)

var _ sv.SecretValueTransformer = (*AzureTransformer)(nil)

type AzureTransformer struct {
}

func (t *AzureTransformer) Transform(ctx context.Context, computedValues map[string]any, value any) (any, error) {
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

	databaseName, ok := computedValues[renderers.DatabaseNameValue].(string)
	if !ok {
		return nil, errors.New("expected the databaseName to be a string")
	}

	u.Path = "/" + databaseName
	return u.String(), nil
}
