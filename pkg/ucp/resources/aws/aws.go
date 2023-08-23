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

package aws

import (
	"strings"

	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	// PlaneTypeAWS defines the type name of the AWS plane.
	PlaneTypeAWS = "aws"

	// ScopeAccounts defines the account scope for AWS resources.
	ScopeAccounts = "accounts"

	// ScopeRegions defines the region scope for AWS resources.
	ScopeRegions = "regions"
)

// ToAWSResourceType takes an ID and returns a string representing the AWS resource type.
func ToAWSResourceType(id resources.ID) string {
	parts := []string{}
	// AWS ARNs use :: as separator.
	for _, segment := range id.TypeSegments() {
		parts = append(parts, strings.ReplaceAll(strings.ReplaceAll(segment.Type, ".", "::"), "/", "::"))
	}
	resourceType := strings.Join(parts, "::")
	return resourceType
}
