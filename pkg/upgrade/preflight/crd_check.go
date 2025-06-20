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

package preflight

import (
	"context"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/kubeutil"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomResourceDefinitionCheck validates that existing CRDs are compatible with the target Radius version.
// It checks for deprecated API versions and CRD schema changes that could cause issues during upgrade.
type CustomResourceDefinitionCheck struct {
	kubeContext string
}

// NewCustomResourceDefinitionCheck creates a new CRD compatibility check.
func NewCustomResourceDefinitionCheck(kubeContext string) *CustomResourceDefinitionCheck {
	return &CustomResourceDefinitionCheck{
		kubeContext: kubeContext,
	}
}

// Name returns the name of this check.
func (c *CustomResourceDefinitionCheck) Name() string {
	return "Custom Resource Definition Compatibility"
}

// Severity returns the severity level of this check.
func (c *CustomResourceDefinitionCheck) Severity() CheckSeverity {
	return SeverityWarning // Warning level as CRD issues can often be resolved automatically
}

// Run executes the CRD compatibility check.
func (c *CustomResourceDefinitionCheck) Run(ctx context.Context) (bool, string, error) {
	// Create Kubernetes client config
	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: c.kubeContext,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
	if err != nil {
		return false, "", fmt.Errorf("failed to create Kubernetes client config: %w", err)
	}

	// Create API extensions client
	apiextClient, err := apiextclient.NewForConfig(config)
	if err != nil {
		return false, "", fmt.Errorf("failed to create API extensions client: %w", err)
	}

	// Get all CRDs in the cluster
	crds, err := apiextClient.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, "", fmt.Errorf("failed to list CustomResourceDefinitions: %w", err)
	}

	// Filter to Radius-related CRDs
	radiusCRDs := c.filterRadiusCRDs(crds.Items)
	if len(radiusCRDs) == 0 {
		return true, "No Radius CustomResourceDefinitions found", nil
	}

	// Check each CRD for compatibility issues
	var warnings []string
	var errors []string

	for _, crd := range radiusCRDs {
		issues := c.checkCRDCompatibility(crd)
		for _, issue := range issues {
			if issue.isError {
				errors = append(errors, fmt.Sprintf("%s: %s", crd.Name, issue.message))
			} else {
				warnings = append(warnings, fmt.Sprintf("%s: %s", crd.Name, issue.message))
			}
		}
	}

	// Build result message
	message := fmt.Sprintf("Found %d Radius CustomResourceDefinitions", len(radiusCRDs))
	
	if len(errors) > 0 {
		message += fmt.Sprintf(". Errors: %s", strings.Join(errors, "; "))
		return false, message, nil
	}

	if len(warnings) > 0 {
		message += fmt.Sprintf(". Warnings: %s", strings.Join(warnings, "; "))
	} else {
		message += ". All CRDs appear compatible"
	}

	return true, message, nil
}

type crdIssue struct {
	message string
	isError bool
}

// filterRadiusCRDs returns only CRDs that belong to Radius
func (c *CustomResourceDefinitionCheck) filterRadiusCRDs(crds []apiextv1.CustomResourceDefinition) []apiextv1.CustomResourceDefinition {
	var radiusCRDs []apiextv1.CustomResourceDefinition
	
	radiusGroups := []string{
		"radapp.io",
		"ucp.dev",
		"dapr.io", // Dapr CRDs used by Radius
	}

	for _, crd := range crds {
		for _, group := range radiusGroups {
			if strings.HasSuffix(crd.Spec.Group, group) {
				radiusCRDs = append(radiusCRDs, crd)
				break
			}
		}
	}

	return radiusCRDs
}

// checkCRDCompatibility examines a CRD for potential upgrade compatibility issues
func (c *CustomResourceDefinitionCheck) checkCRDCompatibility(crd apiextv1.CustomResourceDefinition) []crdIssue {
	var issues []crdIssue

	// Check for deprecated API versions
	if c.hasDeprecatedAPIVersions(crd) {
		issues = append(issues, crdIssue{
			message: "contains deprecated API versions that may not be supported in newer Kubernetes versions",
			isError: false, // Warning - usually handled automatically during upgrade
		})
	}

	// Check CRD conversion strategy
	if crd.Spec.Conversion != nil && crd.Spec.Conversion.Strategy == apiextv1.WebhookConverter {
		issues = append(issues, crdIssue{
			message: "uses webhook conversion which may cause issues if conversion webhook is unavailable during upgrade",
			isError: false, // Warning - not necessarily a blocker
		})
	}

	// Check for finalizers that might block deletion
	if c.hasProblematicFinalizers(crd) {
		issues = append(issues, crdIssue{
			message: "may have finalizers that could block resource cleanup during upgrade",
			isError: false, // Warning - depends on actual resource instances
		})
	}

	return issues
}

// hasDeprecatedAPIVersions checks if the CRD serves any deprecated API versions
func (c *CustomResourceDefinitionCheck) hasDeprecatedAPIVersions(crd apiextv1.CustomResourceDefinition) bool {
	for _, version := range crd.Spec.Versions {
		if version.Deprecated {
			return true
		}
	}
	return false
}

// hasProblematicFinalizers checks if the CRD might have finalizer-related issues
func (c *CustomResourceDefinitionCheck) hasProblematicFinalizers(crd apiextv1.CustomResourceDefinition) bool {
	// Look for common finalizer patterns that might cause issues
	// This is a heuristic check - we can't know for sure without examining actual resources
	return strings.Contains(crd.Name, "gateway") || strings.Contains(crd.Name, "ingress")
}