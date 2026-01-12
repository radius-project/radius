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

package preview

import (
	"fmt"
	"strings"

	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
)

func formatProviderProperties(parts []string) string {
	if len(parts) == 0 {
		return ""
	}

	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(part)
	}

	return b.String()
}

// formatAzureProperties formats Azure provider details for display.
func formatAzureProperties(provider *corerpv20250801.ProvidersAzure) string {
	if provider == nil {
		return ""
	}

	parts := []string{}
	if provider.SubscriptionID != nil {
		parts = append(parts, fmt.Sprintf("subscriptionId: '%s'", *provider.SubscriptionID))
	}
	if provider.ResourceGroupName != nil {
		parts = append(parts, fmt.Sprintf("resourceGroupName: '%s'", *provider.ResourceGroupName))
	}

	return formatProviderProperties(parts)
}

// formatAWSProperties formats AWS provider details for display.
func formatAWSProperties(provider *corerpv20250801.ProvidersAws) string {
	if provider == nil {
		return ""
	}

	parts := []string{}
	if provider.AccountID != nil {
		parts = append(parts, fmt.Sprintf("accountId: '%s'", *provider.AccountID))
	}
	if provider.Region != nil {
		parts = append(parts, fmt.Sprintf("region: '%s'", *provider.Region))
	}

	return formatProviderProperties(parts)
}

// formatKubernetesProperties formats Kubernetes provider details for display.
func formatKubernetesProperties(provider *corerpv20250801.ProvidersKubernetes) string {
	if provider == nil {
		return ""
	}

	parts := []string{}
	if provider.Namespace != nil {
		parts = append(parts, fmt.Sprintf("namespace: '%s'", *provider.Namespace))
	}

	return formatProviderProperties(parts)
}
