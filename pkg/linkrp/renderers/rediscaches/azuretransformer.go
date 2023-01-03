// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/rp"
)

var _ rp.SecretValueTransformer = (*AzureTransformer)(nil)

type AzureTransformer struct {
}

// Transform builds connection string using primary key for Azure Redis Cache resource
func (t *AzureTransformer) Transform(ctx context.Context, computedValues map[string]any, primaryKey any) (any, error) {
	// Redis connection string format: '{hostName}:{port},password={primaryKey},ssl=True,abortConnect=False'
	password, ok := primaryKey.(string)
	if !ok {
		return nil, errors.New("expected the access key to be a string")
	}

	hostname, ok := computedValues[renderers.Host].(string)
	if !ok {
		return nil, errors.New("hostname is required to build Redis connection string")
	}

	port, ok := computedValues[renderers.Port]
	if !ok || port == nil {
		return nil, errors.New("port is required to build Redis connection string")
	}

	connectionString := fmt.Sprintf("%s:%v,password=%s,ssl=True,abortConnect=False", hostname, port, password)

	return connectionString, nil
}
