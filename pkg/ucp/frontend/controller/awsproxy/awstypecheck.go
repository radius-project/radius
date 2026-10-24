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

// createErrorResponse returns a 400 response when Cloud Control cannot create typeName, and nil when it can. It uses
// the provisioning type from CloudFormation, so a type works as soon as AWS adds Cloud Control support for it.
func createErrorResponse(typeName string, typeInfo *cloudformation.DescribeTypeOutput) (armrpc_rest.Response, error) {
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
	return badRequest(fmt.Sprintf("Radius cannot create the AWS resource type %s because AWS Cloud Control does not support it.", typeName)), nil
}

// checkPatch checks a non-empty patch to an existing resource against the provisioning type of typeName. It returns
// the patch to send to Cloud Control, or a 400 response when the type cannot be changed.
//
// A create-and-delete-only (IMMUTABLE) type cannot be updated. AWS never returns write-only properties, so they are
// always in the patch even when they did not change. They are left out of the check and of the returned patch, so a
// deploy that changes nothing else succeeds without calling AWS.
func checkPatch(typeName string, typeInfo *cloudformation.DescribeTypeOutput, patch jsondiff.Patch) (jsondiff.Patch, armrpc_rest.Response, error) {
	switch typeInfo.ProvisioningType {
	case types.ProvisioningTypeNonProvisionable:
		schema, err := parseTypeSchema(typeInfo)
		if err != nil {
			return nil, nil, err
		}
		if schema.isReadOnly() {
			return nil, badRequest(awstypes.ReadOnlyMessage(typeName)), nil
		}
		return nil, badRequest(fmt.Sprintf("Radius cannot change the AWS resource type %s because AWS Cloud Control does not support it.", typeName)), nil

	case types.ProvisioningTypeImmutable:
		schema, err := parseTypeSchema(typeInfo)
		if err != nil {
			return nil, nil, err
		}
		changed := []string{}
		for _, op := range patch {
			if schema.isWriteOnly(op.Path) {
				continue
			}
			if name := topLevelProperty(op.Path); !slices.Contains(changed, name) {
				changed = append(changed, name)
			}
		}
		if len(changed) == 0 {
			return nil, nil, nil
		}
		slices.Sort(changed)
		return nil, badRequest(fmt.Sprintf("The AWS resource type %s can only be created and deleted, so these properties of the existing resource cannot change: %s. To change them, delete the resource and deploy again.", typeName, strings.Join(changed, ", "))), nil

	default:
		return patch, nil, nil
	}
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

// isWriteOnly reports whether a JSON patch path is a write-only property or is inside one.
func (s *typeSchema) isWriteOnly(patchPath string) bool {
	path := "/properties" + patchPath
	return slices.ContainsFunc(s.WriteOnlyProperties, func(w string) bool {
		return path == w || strings.HasPrefix(path, w+"/")
	})
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
