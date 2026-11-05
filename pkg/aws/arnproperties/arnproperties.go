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

// Package arnproperties knows which property of an AWS resource holds the ARN of the resource itself, and how to
// store that ARN so every recipe driver records the same value for the same resource.
package arnproperties

//go:generate go run ./gen

import (
	"strings"
	"sync"

	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"

	"github.com/radius-project/radius/pkg/aws/schemas"
)

var arnPropertiesByLowerType = sync.OnceValue(func() map[string]string {
	// The schemas are built into Radius, and a test in pkg/aws/schemas loads them. If they failed to load, the catalog
	// would have no metadata, and only the generated catalog would be used.
	catalog, _ := schemas.Load()
	return byLowerType(arnProperties, catalog.Metadata)
})

// byLowerType returns generated, keyed by the lowercase type, with the arnProperty of each type in metadata that
// declares one. Hand-written types aren't in the generated catalog, and metadata overrides it for the others.
func byLowerType(generated map[string]string, metadata map[string]schemas.Metadata) map[string]string {
	result := map[string]string{}
	for resourceType, property := range generated {
		result[strings.ToLower(resourceType)] = property
	}
	for resourceType, m := range metadata {
		if m.ARNProperty != nil {
			result[strings.ToLower(resourceType)] = *m.ARNProperty
		}
	}
	return result
}

// Lookup returns the property that holds the ARN of a resource of the given CloudFormation type, such as "TopicArn"
// for "AWS::SNS::Topic". The type is matched case-insensitively. A type's arnProperty in its metadata in
// pkg/aws/schemas takes precedence over the generated catalog.
//
// known is false when the type isn't in the catalog. When known is true and property is empty, the type has no
// property with the resource's own ARN.
func Lookup(resourceType string) (property string, known bool) {
	property, known = arnPropertiesByLowerType()[strings.ToLower(resourceType)]
	return property, known
}

// ProviderResourceID returns the provider resource ID to store for an AWS resource with the given ARN.
//
// It is the ARN, except for a CloudWatch Logs log group ARN in the form "arn:<partition>:logs:<region>:<account>:
// log-group:<name>:*". Cloud Control returns that form, but the Terraform AWS provider removes the trailing ":*", so
// both are stored without it. No other ARN is changed, because other qualifiers can tell distinct resources apart.
func ProviderResourceID(arn string) string {
	parsed, err := awsarn.Parse(arn)
	if err != nil || parsed.Service != "logs" {
		return arn
	}

	name, ok := strings.CutPrefix(parsed.Resource, "log-group:")
	if !ok {
		return arn
	}
	name, ok = strings.CutSuffix(name, ":*")
	if !ok || name == "" || strings.Contains(name, ":") {
		return arn
	}

	return strings.TrimSuffix(arn, ":*")
}
