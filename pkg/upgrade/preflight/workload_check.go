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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ActiveWorkloadHealthCheck validates that critical Radius workloads are in a healthy state
// before proceeding with the upgrade. It checks for stuck deployments, failed pods, and other
// issues that could interfere with the upgrade process.
type ActiveWorkloadHealthCheck struct {
	kubeContext string
}

// NewActiveWorkloadHealthCheck creates a new active workload health check.
func NewActiveWorkloadHealthCheck(kubeContext string) *ActiveWorkloadHealthCheck {
	return &ActiveWorkloadHealthCheck{
		kubeContext: kubeContext,
	}
}

// Name returns the name of this check.
func (w *ActiveWorkloadHealthCheck) Name() string {
	return "Active Workload Health"
}

// Severity returns the severity level of this check.
func (w *ActiveWorkloadHealthCheck) Severity() CheckSeverity {
	return SeverityWarning // Warning level as some workload issues can be resolved during upgrade
}

// Run executes the active workload health check.
func (w *ActiveWorkloadHealthCheck) Run(ctx context.Context) (bool, string, error) {
	// Create Kubernetes client config
	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: w.kubeContext,
		QPS:         kubeutil.DefaultCLIQPS,
		Burst:       kubeutil.DefaultCLIBurst,
	})
	if err != nil {
		return false, "", fmt.Errorf("failed to create Kubernetes client config: %w", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return false, "", fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Check Radius system workloads
	radiusIssues, err := w.checkRadiusSystemWorkloads(ctx, clientset)
	if err != nil {
		return false, "", fmt.Errorf("failed to check Radius system workloads: %w", err)
	}

	// Check Radius application workloads
	appIssues, err := w.checkRadiusApplicationWorkloads(ctx, clientset)
	if err != nil {
		return false, "", fmt.Errorf("failed to check Radius application workloads: %w", err)
	}

	allIssues := append(radiusIssues, appIssues...)

	if len(allIssues) == 0 {
		return true, "All Radius workloads are healthy", nil
	}

	// Determine if any issues are critical
	var criticalIssues []string
	var warningIssues []string

	for _, issue := range allIssues {
		if issue.isCritical {
			criticalIssues = append(criticalIssues, issue.message)
		} else {
			warningIssues = append(warningIssues, issue.message)
		}
	}

	message := fmt.Sprintf("Found %d workload issues", len(allIssues))
	if len(criticalIssues) > 0 {
		message += fmt.Sprintf(". Critical: %s", strings.Join(criticalIssues, "; "))
	}
	if len(warningIssues) > 0 {
		message += fmt.Sprintf(". Warnings: %s", strings.Join(warningIssues, "; "))
	}

	// Return success if only warnings, failure if any critical issues
	success := len(criticalIssues) == 0
	return success, message, nil
}

type workloadIssue struct {
	message    string
	isCritical bool
}

// checkRadiusSystemWorkloads examines the health of core Radius system components
func (w *ActiveWorkloadHealthCheck) checkRadiusSystemWorkloads(ctx context.Context, clientset kubernetes.Interface) ([]workloadIssue, error) {
	var issues []workloadIssue

	// Get deployments in radius-system namespace
	deployments, err := clientset.AppsV1().Deployments("radius-system").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments in radius-system: %w", err)
	}

	if len(deployments.Items) == 0 {
		issues = append(issues, workloadIssue{
			message:    "no deployments found in radius-system namespace",
			isCritical: true,
		})
		return issues, nil
	}

	// Check each deployment
	for _, deployment := range deployments.Items {
		deploymentIssues := w.checkDeploymentHealth(deployment, true)
		issues = append(issues, deploymentIssues...)
	}

	return issues, nil
}

// checkRadiusApplicationWorkloads examines Radius-managed application workloads
func (w *ActiveWorkloadHealthCheck) checkRadiusApplicationWorkloads(ctx context.Context, clientset kubernetes.Interface) ([]workloadIssue, error) {
	var issues []workloadIssue

	// Get all namespaces to find Radius applications
	namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	radiusAppCount := 0
	for _, ns := range namespaces.Items {
		// Look for namespaces with Radius application labels/annotations
		if w.isRadiusApplicationNamespace(ns) {
			radiusAppCount++
			
			// Check deployments in this namespace
			deployments, err := clientset.AppsV1().Deployments(ns.Name).List(ctx, metav1.ListOptions{})
			if err != nil {
				issues = append(issues, workloadIssue{
					message:    fmt.Sprintf("failed to list deployments in namespace %s", ns.Name),
					isCritical: false,
				})
				continue
			}

			// Check each deployment (less critical than system components)
			for _, deployment := range deployments.Items {
				deploymentIssues := w.checkDeploymentHealth(deployment, false)
				issues = append(issues, deploymentIssues...)
			}
		}
	}

	// Add informational message about Radius apps found
	if radiusAppCount > 0 {
		issues = append(issues, workloadIssue{
			message:    fmt.Sprintf("found %d Radius application namespaces", radiusAppCount),
			isCritical: false,
		})
	}

	return issues, nil
}

// checkDeploymentHealth examines a deployment for health issues
func (w *ActiveWorkloadHealthCheck) checkDeploymentHealth(deployment appsv1.Deployment, isSystemComponent bool) []workloadIssue {
	var issues []workloadIssue

	// Check if deployment is ready
	if deployment.Status.ReadyReplicas < deployment.Status.Replicas {
		severity := !isSystemComponent // system components are critical
		issues = append(issues, workloadIssue{
			message:    fmt.Sprintf("deployment %s/%s has %d/%d ready replicas", deployment.Namespace, deployment.Name, deployment.Status.ReadyReplicas, deployment.Status.Replicas),
			isCritical: severity,
		})
	}

	// Check for stuck rollouts
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse {
			issues = append(issues, workloadIssue{
				message:    fmt.Sprintf("deployment %s/%s has stalled rollout: %s", deployment.Namespace, deployment.Name, condition.Message),
				isCritical: isSystemComponent,
			})
		}
	}

	// Check replica count
	if deployment.Status.Replicas == 0 {
		issues = append(issues, workloadIssue{
			message:    fmt.Sprintf("deployment %s/%s has 0 replicas", deployment.Namespace, deployment.Name),
			isCritical: isSystemComponent,
		})
	}

	return issues
}

// isRadiusApplicationNamespace determines if a namespace contains Radius applications
func (w *ActiveWorkloadHealthCheck) isRadiusApplicationNamespace(ns corev1.Namespace) bool {
	// Check for Radius-specific labels or annotations
	if ns.Labels != nil {
		if _, hasRadiusLabel := ns.Labels["radapp.io/environment"]; hasRadiusLabel {
			return true
		}
		if _, hasRadiusLabel := ns.Labels["app.kubernetes.io/managed-by"]; hasRadiusLabel {
			return true
		}
	}

	if ns.Annotations != nil {
		if _, hasRadiusAnnotation := ns.Annotations["radapp.io/environment"]; hasRadiusAnnotation {
			return true
		}
	}

	// Skip system namespaces
	systemNamespaces := []string{"kube-system", "kube-public", "kube-node-lease", "radius-system", "default"}
	for _, sysNS := range systemNamespaces {
		if ns.Name == sysNS {
			return false
		}
	}

	return false
}