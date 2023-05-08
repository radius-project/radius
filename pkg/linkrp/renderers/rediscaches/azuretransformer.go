/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package rediscaches

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/linkrp/renderers"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
)

var _ sv.SecretValueTransformer = (*AzureTransformer)(nil)

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
