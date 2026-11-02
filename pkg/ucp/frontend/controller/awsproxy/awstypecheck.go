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

// checkPatch checks a non-empty patch to an existing resource. It returns the patch to send to Cloud Control, or a
// 400 response when a create-and-delete-only (IMMUTABLE) type would change.
//
// AWS never returns write-only properties, so they are always in the patch, even when they did not change. UCP cannot
// tell whether a write-only value changed, so a patch to an IMMUTABLE type that sets only write-only properties also
// fails, rather than succeeding without applying a changed value.
func checkPatch(typeName string, typeInfo *cloudformation.DescribeTypeOutput, patch jsondiff.Patch) (jsondiff.Patch, armrpc_rest.Response, error) {
	if typeInfo.ProvisioningType != types.ProvisioningTypeImmutable {
		return patch, nil, nil
	}
	schema, err := parseTypeSchema(typeInfo)
	if err != nil {
		return nil, nil, err
	}
	changed, writeOnly := []string{}, []string{}
	for _, op := range patch {
		name := topLevelProperty(op.Path)
		if schema.onlyWriteOnly(op) {
			if !slices.Contains(writeOnly, name) {
				writeOnly = append(writeOnly, name)
			}
			continue
		}
		if !slices.Contains(changed, name) {
			changed = append(changed, name)
		}
	}
	if len(changed) == 0 {
		slices.Sort(writeOnly)
		return nil, badRequest(fmt.Sprintf("The AWS resource type %s can only be created and deleted, and AWS cannot update it in place. Radius cannot confirm that these write-only properties of the existing resource are unchanged, because AWS does not return their values: %s. To change them, delete the resource and deploy again.", typeName, strings.Join(writeOnly, ", "))), nil
	}
	slices.Sort(changed)
	return nil, badRequest(fmt.Sprintf("The AWS resource type %s can only be created and deleted, so these properties of the existing resource cannot change: %s. To change them, delete the resource and deploy again.", typeName, strings.Join(changed, ", "))), nil
}

// typeSchema is the part of a CloudFormation resource type schema that the type checks need.
type typeSchema struct {
	Handlers            map[string]json.RawMessage `json:"handlers"`
	WriteOnlyProperties []string                   `json:"writeOnlyProperties"`
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

// isWriteOnly reports whether a JSON patch path is a write-only property or is inside one. A "*" segment in a
// write-only property matches any array index.
func (s *typeSchema) isWriteOnly(patchPath string) bool {
	path := strings.Split(strings.TrimPrefix(patchPath, "/"), "/")
	return slices.ContainsFunc(s.WriteOnlyProperties, func(w string) bool {
		pattern := strings.Split(strings.TrimPrefix(w, "/properties/"), "/")
		if len(path) < len(pattern) {
			return false
		}
		for i, segment := range pattern {
			if segment != "*" && segment != path[i] {
				return false
			}
		}
		return true
	})
}

// onlyWriteOnly reports whether a patch operation sets only write-only properties. When AWS omits an object because
// all of its properties are write-only, the patch adds the whole object, so every leaf of the added value is checked.
func (s *typeSchema) onlyWriteOnly(op jsondiff.Operation) bool {
	if s.isWriteOnly(op.Path) {
		return true
	}
	if op.Type != jsondiff.OperationAdd && op.Type != jsondiff.OperationReplace {
		return false
	}
	leaves := leafPaths(op.Path, op.Value)
	return !slices.ContainsFunc(leaves, func(p string) bool { return !s.isWriteOnly(p) })
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
