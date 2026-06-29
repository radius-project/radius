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

package frontend

import (
	"context"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// redactionPaths holds the field paths a resource schema marks for redaction on read.
//
//   - sensitive: every field marked x-radius-sensitive.
//   - retain: the subset of sensitive fields also marked x-radius-retain. Retain fields keep their
//     encrypted value at rest (vault semantics) instead of being redacted to nil by the backend, so
//     they are the only sensitive fields still populated once a resource reaches Succeeded and MUST
//     be redacted on read so the API never returns the retained ciphertext.
type redactionPaths struct {
	sensitive []string
	retain    []string
}

// fetchRedactionPaths fetches the schema for a resource type/api-version once and extracts both the
// sensitive and retain field paths from it. Returning empty paths (with no error) when the schema is
// unavailable mirrors GetSensitiveFieldPaths.
func fetchRedactionPaths(ctx context.Context, ucpClient *v20231001preview.ClientFactory, resourceID string, resourceType string, apiVersion string) (redactionPaths, error) {
	schemaMap, err := schema.GetSchema(ctx, ucpClient, resourceID, resourceType, apiVersion)
	if err != nil {
		return redactionPaths{}, err
	}
	if schemaMap == nil {
		return redactionPaths{}, nil
	}

	return redactionPaths{
		sensitive: schema.ExtractSensitiveFieldPaths(schemaMap, ""),
		retain:    schema.ExtractRetainFieldPaths(schemaMap, ""),
	}, nil
}

// forState returns the field paths to redact for a resource in the given provisioning state.
//
// At Succeeded the backend has already redacted every non-retain sensitive field to nil, so only the
// retain fields (now holding ciphertext) remain to be redacted. In any other state the resource may
// still carry encrypted values for any sensitive field, so all sensitive fields are redacted.
func (p redactionPaths) forState(state v1.ProvisioningState) []string {
	if state == v1.ProvisioningStateSucceeded {
		return p.retain
	}
	return p.sensitive
}
