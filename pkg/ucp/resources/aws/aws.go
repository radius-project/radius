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
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/resources"
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

// ToUCPResourceID takes AWS resource ARN and returns string representing UCP qualified resource ID.
// General formats for ARNs: https://docs.aws.amazon.com/IAM/latest/UserGuide/reference-arns.html
func ToUCPResourceID(arn string) (string, error) {
	arnSegments := strings.Split(arn, ":")
	if len(arnSegments) < 6 {
		return "", fmt.Errorf("\"%s\" is not a valid ARN", arn)
	}

	service := arnSegments[2]
	region := arnSegments[3]
	account := arnSegments[4]
	resourcePath := strings.Join(arnSegments[5:], "/")
	ucpID := ""
	// Handle global services that don't have regions (like IAM policies, users, groups, route53, cloudfront etc.)
	if region == "" {
		region = "global"
	}
	ucpID = fmt.Sprintf("/planes/aws/%s/accounts/%s/regions/%s/providers/AWS.%s/%s", arnSegments[1], account, region, service, resourcePath)

	return ucpID, nil
}
