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

package awsproxy

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/wI2L/jsondiff"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/aws/awstypes"
)

// Error messages name the type and properties, never property values, because values can hold secrets.

// unsupportedTypeResponse returns a 400 response for a legacy or deprecated AWS type, and nil for any other type.
// Callers check this before they call AWS.
func unsupportedTypeResponse(typeName string) armrpc_rest.Response {
	message, ok := awstypes.UnsupportedMessage(typeName)
	if !ok {
		return nil
	}
	return badRequest(message)
}

// putErrorResponse returns a 400 response when Cloud Control cannot create or update typeName, and nil when it can.
// It uses the provisioning type from CloudFormation, so a type works as soon as AWS adds Cloud Control support for it.
// Callers check this before they look for an existing resource, because Cloud Control rejects that read for most
// NON_PROVISIONABLE types.
func putErrorResponse(typeName string, typeInfo *cloudformation.DescribeTypeOutput) (armrpc_rest.Response, error) {
	if typeInfo.ProvisioningType != types.ProvisioningTypeNonProvisionable {
		return nil, nil
	}
	schema, err := parseTypeSchema(typeInfo)
	if err != nil {
		return nil, err
	}
	if schema.isReadOnly() {
		return badRequest(awstypes.ReadOnlyMessage(typeName)), nil
	}
	return badRequest(fmt.Sprintf("Radius cannot create or change the AWS resource type %s because AWS Cloud Control does not support it.", typeName)), nil
}

// checkPatch checks a non-empty patch to an existing resource. It returns a 400 response when the type is
// create-and-delete-only (IMMUTABLE), and nil otherwise. Write-only values that UCP does not send, such as values whose
// fingerprint shows they are unchanged, are already out of the patch, so a write-only value in it changed.
func checkPatch(typeName string, typeInfo *cloudformation.DescribeTypeOutput, patch jsondiff.Patch) armrpc_rest.Response {
	if typeInfo.ProvisioningType != types.ProvisioningTypeImmutable {
		return nil
	}
	changed := []string{}
	for _, op := range patch {
		if name := topLevelProperty(op.Path); !slices.Contains(changed, name) {
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)
	return badRequest(fmt.Sprintf("The AWS resource type %s can only be created and deleted, so these properties of the existing resource cannot change: %s. To change them, delete the resource and deploy again.", typeName, strings.Join(changed, ", ")))
}

// typeSchema is the part of a CloudFormation resource type schema that the type checks need.
type typeSchema struct {
	Handlers             map[string]json.RawMessage `json:"handlers"`
	CreateOnlyProperties []string                   `json:"createOnlyProperties"`
	WriteOnlyProperties  []string                   `json:"writeOnlyProperties"`
}

func parseTypeSchema(typeInfo *cloudformation.DescribeTypeOutput) (*typeSchema, error) {
	schema := &typeSchema{}
	if err := json.Unmarshal([]byte(aws.ToString(typeInfo.Schema)), schema); err != nil {
		return nil, fmt.Errorf("failed to read the CloudFormation schema: %w", err)
	}
	return schema, nil
}

// isReadOnly reports whether the schema describes a type that AWS creates on its own: it can be read but not created.
func (s *typeSchema) isReadOnly() bool {
	_, read := s.Handlers["read"]
	_, create := s.Handlers["create"]
	return read && !create
}

// writeOnlyPattern returns the write-only property that a JSON patch path is or is inside, relative to the properties
// object, such as Users/*/Secret. A "*" segment in a write-only property matches any array index.
func (s *typeSchema) writeOnlyPattern(patchPath string) (string, bool) {
	path := strings.Split(strings.TrimPrefix(patchPath, "/"), "/")
	for _, w := range s.WriteOnlyProperties {
		pattern := strings.TrimPrefix(w, "/properties/")
		if matchesPrefix(path, strings.Split(pattern, "/")) {
			return pattern, true
		}
	}
	return "", false
}

// matchesPrefix reports whether pattern matches the first segments of path. A "*" segment matches any segment.
func matchesPrefix(path []string, pattern []string) bool {
	if len(path) < len(pattern) {
		return false
	}
	for i, segment := range pattern {
		if segment != "*" && segment != path[i] {
			return false
		}
	}
	return true
}

// leafPaths returns the JSON patch paths of the values inside value, which is at path. An empty object or array is a
// leaf itself.
func leafPaths(path string, value any) []string {
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			return []string{path}
		}
		escape := strings.NewReplacer("~", "~0", "/", "~1")
		paths := []string{}
		for key, child := range v {
			paths = append(paths, leafPaths(path+"/"+escape.Replace(key), child)...)
		}
		return paths
	case []any:
		if len(v) == 0 {
			return []string{path}
		}
		paths := []string{}
		for i, child := range v {
			paths = append(paths, leafPaths(fmt.Sprintf("%s/%d", path, i), child)...)
		}
		return paths
	default:
		return []string{path}
	}
}

// topLevelProperty returns the name of the top-level property a JSON patch path points into.
func topLevelProperty(patchPath string) string {
	name, _, _ := strings.Cut(strings.TrimPrefix(patchPath, "/"), "/")
	return strings.NewReplacer("~1", "/", "~0", "~").Replace(name)
}

func badRequest(message string) armrpc_rest.Response {
	return armrpc_rest.NewBadRequestARMResponse(v1.ErrorResponse{
		Error: &v1.ErrorDetails{
			Code:    v1.CodeInvalid,
			Message: message,
		},
	})
}
