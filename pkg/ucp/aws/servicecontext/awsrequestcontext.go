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

package servicecontext

import (
	"context"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// AWSRequestContext is the context for AWS request.
type AWSRequestContext struct {
	// AWSRequestContext has all the fields from ARMRequestContext.
	*v1.ARMRequestContext
}

// ARMRequestContextFromContext extracts AWS Request Context from http context.
func AWSRequestContextFromContext(ctx context.Context) *AWSRequestContext {
	return &AWSRequestContext{v1.ARMRequestContextFromContext(ctx)}
}

// ResourceTypeInAWSFormat returns the AWS resource type.
func (c *AWSRequestContext) ResourceTypeInAWSFormat() string {
	return resources.ToAWSResourceType(c.ResourceID)
}
