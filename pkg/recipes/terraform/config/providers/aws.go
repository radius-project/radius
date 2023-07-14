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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/resources"
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
func (p *awsProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error) {
	if (envConfig == nil) || (envConfig.Providers == datamodel.Providers{}) || (envConfig.Providers.AWS == datamodel.ProvidersAWS{}) || envConfig.Providers.AWS.Scope == "" {
		return nil, v1.NewClientErrInvalidRequest("AWS provider is required to be configured on the Environment to create AWS resources using Recipe")
	}

	region, err := parseAWSScope(envConfig.Providers.AWS.Scope)
	if err != nil {
		return nil, err
	}

	awsConfig := map[string]any{
		"region": region,
	}

	return awsConfig, nil
}

func parseAWSScope(scope string) (region string, err error) {
	parsedScope, err := resources.Parse(scope)
	if err != nil {
		return "", v1.NewClientErrInvalidRequest(fmt.Sprintf("error parsing AWS scope %q: %s", scope, err.Error()))
	}

	for _, segment := range parsedScope.ScopeSegments() {
		if strings.EqualFold(segment.Type, resources.RegionsSegment) {
			region = segment.Name
		}
	}

	return region, nil
}
