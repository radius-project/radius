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

package util

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	resources "github.com/radius-project/radius/pkg/ucp/resources"
)

// FetchScopeResource checks if the given scopeID is a valid resource ID for the given resource type, fetches the resource
// from the database client and returns an error if the resource does not exist.
func FetchScopeResource(ctx context.Context, databaseClient database.Client, scopeID string, resource v1.DataModelInterface) error {
	id, err := resources.ParseResource(scopeID)
	if err != nil {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("%s is not a valid resource id for %s.", scopeID, resource.ResourceTypeName()))
	}

	if !strings.EqualFold(id.Type(), resource.ResourceTypeName()) {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("linked %q has invalid %s resource type.", scopeID, resource.ResourceTypeName()))
	}

	res, err := databaseClient.Get(ctx, id.String())
	if errors.Is(&database.ErrNotFound{ID: id.String()}, err) {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("linked resource %s does not exist", scopeID))
	}
	if err != nil {
		return fmt.Errorf("failed to fetch %s. Error: %w", scopeID, err)
	}

	err = res.As(resource)
	if err != nil {
		return err
	}

	return nil
}
