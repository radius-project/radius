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

package reconciler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/upgrade/preflight"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// RadiusChartName is the name of the Radius Helm chart
	RadiusChartName = "radius"
	// RadiusPreflightAnnotation tracks the last version we ran preflight checks for
	RadiusPreflightAnnotation = "radius.io/preflight-checked-version"
	// RadiusPreflightHoldAnnotation indicates the HelmRelease is on hold due to preflight failures
	RadiusPreflightHoldAnnotation = "radius.io/preflight-hold"
)

// FluxHelmReleaseReconciler watches Flux HelmRelease objects for Radius upgrades
type FluxHelmReleaseReconciler struct {
	Client            client.Client
	Scheme            *runtime.Scheme
	EventRecorder     record.EventRecorder
	PreflightRegistry *preflight.Registry
}

// Reconcile processes Flux HelmRelease objects to detect Radius upgrades
func (r *FluxHelmReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Use unstructured to work with Flux HelmRelease objects
	helmRelease := &unstructured.Unstructured{}
	helmRelease.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "helm.toolkit.fluxcd.io",
		Version: "v2",
		Kind:    "HelmRelease",
	})

	if err := r.Client.Get(ctx, req.NamespacedName, helmRelease); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Only process Radius charts
	if !r.isRadiusChart(helmRelease) {
		return ctrl.Result{}, nil
	}

	// Check if we need to run preflight checks
	chartVersion := r.getChartVersion(helmRelease)
	if chartVersion == "" {
		return ctrl.Result{}, nil
	}

	lastCheckedVersion := r.getAnnotation(helmRelease, RadiusPreflightAnnotation)
	if lastCheckedVersion == chartVersion {
		return ctrl.Result{}, nil // Already processed this version
	}

	// Get current deployed version from the HelmRelease status
	currentVersion := r.getCurrentDeployedVersion(helmRelease)

	// Run preflight checks
	logger.Info("Running preflight checks for Radius upgrade",
		"currentVersion", currentVersion, "targetVersion", chartVersion)

	if err := r.runPreflightChecks(ctx, currentVersion, chartVersion); err != nil {
		r.EventRecorder.Event(helmRelease, "Warning", "PreflightFailed",
			fmt.Sprintf("Preflight checks failed: %v", err))
		
		// Put HelmRelease on hold to prevent Flux from proceeding with the upgrade
		if holdErr := r.holdHelmRelease(ctx, helmRelease, err.Error()); holdErr != nil {
			logger.Error(holdErr, "Failed to put HelmRelease on hold")
		}
		
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	// Remove any hold and mark as processed
	if err := r.clearHoldAndMarkComplete(ctx, helmRelease, chartVersion); err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(helmRelease, "Normal", "PreflightSuccess",
		fmt.Sprintf("Preflight checks passed for version %s", chartVersion))

	return ctrl.Result{}, nil
}

// isRadiusChart checks if this HelmRelease is for a Radius chart
func (r *FluxHelmReleaseReconciler) isRadiusChart(hr *unstructured.Unstructured) bool {
	chartName := r.getNestedString(hr, "spec", "chart", "spec", "chart")
	releaseName := r.getNestedString(hr, "spec", "releaseName")

	// Check for various Radius chart patterns:
	// 1. Chart name contains "radius"
	// 2. Release name is "radius"
	// 3. Chart path is the standard Radius chart path
	return strings.Contains(chartName, RadiusChartName) ||
		releaseName == RadiusChartName ||
		chartName == "./deploy/Chart" // Standard Radius chart path in GitRepository
}

// getChartVersion extracts the chart version from the HelmRelease
func (r *FluxHelmReleaseReconciler) getChartVersion(hr *unstructured.Unstructured) string {
	return r.getNestedString(hr, "spec", "chart", "spec", "version")
}

// getAnnotation gets an annotation value
func (r *FluxHelmReleaseReconciler) getAnnotation(hr *unstructured.Unstructured, key string) string {
	annotations := hr.GetAnnotations()
	if annotations == nil {
		return ""
	}
	return annotations[key]
}

// getCurrentDeployedVersion gets the currently deployed version from HelmRelease status
func (r *FluxHelmReleaseReconciler) getCurrentDeployedVersion(hr *unstructured.Unstructured) string {
	// Try to get from status.history[0].chartVersion (most recent deployment)
	history, found, err := unstructured.NestedSlice(hr.Object, "status", "history")
	if found && err == nil && len(history) > 0 {
		if historyItem, ok := history[0].(map[string]any); ok {
			if chartVersion, ok := historyItem["chartVersion"].(string); ok {
				return chartVersion
			}
		}
	}

	// Fallback to lastAttemptedRevision
	return r.getNestedString(hr, "status", "lastAttemptedRevision")
}

// getNestedString safely extracts a nested string from unstructured data
func (r *FluxHelmReleaseReconciler) getNestedString(hr *unstructured.Unstructured, fields ...string) string {
	value, found, err := unstructured.NestedString(hr.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return value
}

// runPreflightChecks executes all registered preflight checks
func (r *FluxHelmReleaseReconciler) runPreflightChecks(ctx context.Context, currentVersion, targetVersion string) error {
	if r.PreflightRegistry == nil {
		return nil // No checks configured
	}

	// Create a new registry with version-specific checks
	// Note: We create a new registry instance to avoid conflicts with other reconciler instances
	tempRegistry := preflight.NewRegistry(r.PreflightRegistry.GetOutput())

	// Add version compatibility check with actual versions
	// Skip version check if versions are the same (graceful handling for GitOps re-processing)
	if currentVersion != "" && targetVersion != "" && currentVersion != targetVersion {
		tempRegistry.AddCheck(preflight.NewVersionCompatibilityCheck(currentVersion, targetVersion))
	}

	// Run all checks - the registry handles the execution
	_, err := tempRegistry.RunChecks(ctx)
	return err
}

// holdHelmRelease puts the HelmRelease on hold to prevent Flux from proceeding
func (r *FluxHelmReleaseReconciler) holdHelmRelease(ctx context.Context, hr *unstructured.Unstructured, reason string) error {
	annotations := hr.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	
	// Add hold annotation with reason
	annotations[RadiusPreflightHoldAnnotation] = reason
	
	// Suspend the HelmRelease by setting spec.suspend = true
	spec, found, err := unstructured.NestedMap(hr.Object, "spec")
	if !found || err != nil {
		spec = make(map[string]any)
	}
	spec["suspend"] = true
	hr.Object["spec"] = spec
	
	hr.SetAnnotations(annotations)
	return r.Client.Update(ctx, hr)
}

// clearHoldAndMarkComplete removes hold and marks preflight as complete
func (r *FluxHelmReleaseReconciler) clearHoldAndMarkComplete(ctx context.Context, hr *unstructured.Unstructured, version string) error {
	annotations := hr.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	
	// Remove hold annotation and mark preflight complete
	delete(annotations, RadiusPreflightHoldAnnotation)
	annotations[RadiusPreflightAnnotation] = version
	
	// Resume the HelmRelease by removing spec.suspend or setting it to false
	spec, found, err := unstructured.NestedMap(hr.Object, "spec")
	if found && err == nil {
		delete(spec, "suspend") // Remove suspend field entirely (default is false)
		hr.Object["spec"] = spec
	}
	
	hr.SetAnnotations(annotations)
	return r.Client.Update(ctx, hr)
}

// SetupWithManager sets up the controller with the Manager
func (r *FluxHelmReleaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Use unstructured to watch HelmRelease objects
	helmRelease := &unstructured.Unstructured{}
	helmRelease.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "helm.toolkit.fluxcd.io",
		Version: "v2",
		Kind:    "HelmRelease",
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(helmRelease).
		Complete(r)
}
