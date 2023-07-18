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

package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

const (
	AWSProviderName = "aws"
)

type awsProvider struct{}

// NewAWSProvider creates a new AWSProvider instance.
func NewAWSProvider() Provider {
	return &awsProvider{}
}

// BuildConfig generates the Terraform provider configuration for AWS provider.
// https://registry.terraform.io/providers/hashicorp/aws/latest/docs
func (p *awsProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) map[string]any {
	logger := ucplog.FromContextOrDiscard(ctx)
	awsConfig := make(map[string]any)
	if (envConfig == nil) || (envConfig.Providers == datamodel.Providers{}) || (envConfig.Providers.AWS == datamodel.ProvidersAWS{}) || envConfig.Providers.AWS.Scope == "" {
		logger.Info("AWS provider scope is not configured on the Environment, skipping AWS region configuration.")
		return awsConfig
	}

	region := parseAWSScope(ctx, envConfig.Providers.AWS.Scope)
	if region != "" {
		awsConfig = map[string]any{
			"region": region,
		}
	}

	return awsConfig
}

func parseAWSScope(ctx context.Context, scope string) (region string) {
	logger := ucplog.FromContextOrDiscard(ctx)
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		logger.Info(fmt.Sprintf("Invalid AWS provider scope is configured on the Environment, error parsing: %s", err.Error()))
		return ""
	}

	for _, segment := range parsedScope.ScopeSegments() {
		if strings.EqualFold(segment.Type, resources.RegionsSegment) {
			region = segment.Name
		}
	}

	return region
}
