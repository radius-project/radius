// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
