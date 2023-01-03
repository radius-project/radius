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

var _ rp.SecretValueTransformer = (*AzureConnectionStringTransformer)(nil)

type AzureConnectionStringTransformer struct {
}

// Transform builds connection string using primary key for Azure Redis Cache resource
func (t *AzureConnectionStringTransformer) Transform(ctx context.Context, computedValues map[string]interface{}, primaryKey interface{}) (interface{}, error) {
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

var _ rp.SecretValueTransformer = (*AzureURLTransformer)(nil)

type AzureURLTransformer struct {
}

// Transform builds connection string using primary key for Azure Redis Cache resource
func (t *AzureURLTransformer) Transform(ctx context.Context, computedValues map[string]interface{}, primaryKey interface{}) (interface{}, error) {
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

	username, ok := computedValues[renderers.UsernameStringValue]
	if !ok {
		username = "" // Blank username is ok
	}

	url := fmt.Sprintf("rediss://%s:%s@%s:%v", username, password, hostname, port)
	return url, nil
}
