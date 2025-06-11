/*
Copyright 2025 The Radius Authors.

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
	// RadiusUpgradeEnabledAnnotation marks a HelmRelease for upgrade preflight checks
	RadiusUpgradeEnabledAnnotation = "radapp.io/upgrade-enabled"
	// RadiusUpgradeCheckedAnnotation tracks the last version we ran preflight checks for
	RadiusUpgradeCheckedAnnotation = "radapp.io/upgrade-checked-version"
	// RadiusUpgradeHoldAnnotation indicates the HelmRelease is on hold due to preflight failures
	RadiusUpgradeHoldAnnotation = "radapp.io/upgrade-hold"
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

	lastCheckedVersion := r.getAnnotation(helmRelease, RadiusUpgradeCheckedAnnotation)
	if lastCheckedVersion == chartVersion {
		return ctrl.Result{}, nil
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

// isRadiusChart checks if this HelmRelease is opted-in for Radius upgrade preflight checks
func (r *FluxHelmReleaseReconciler) isRadiusChart(hr *unstructured.Unstructured) bool {
	// Check for explicit opt-in annotation
	annotations := hr.GetAnnotations()
	if annotations != nil {
		if enabled, exists := annotations[RadiusUpgradeEnabledAnnotation]; exists && enabled == "true" {
			return true
		}
	}
	return false
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
		return nil
	}

	// Create a new registry with version-specific checks
	tempRegistry := preflight.NewRegistry(r.PreflightRegistry.GetOutput())

	// Add version compatibility check if both versions are specified
	// and they are different
	if currentVersion != "" && targetVersion != "" && currentVersion != targetVersion {
		tempRegistry.AddCheck(preflight.NewVersionCompatibilityCheck(currentVersion, targetVersion))
	}

	_, err := tempRegistry.RunChecks(ctx)
	return err
}

// holdHelmRelease puts the HelmRelease on hold to prevent Flux from proceeding
// it does this by adding the `spec.suspend=true` annotation
// https://v2-0.docs.fluxcd.io/flux/components/helm/api/
func (r *FluxHelmReleaseReconciler) holdHelmRelease(ctx context.Context, hr *unstructured.Unstructured, reason string) error {
	annotations := hr.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Add hold annotation with reason
	annotations[RadiusUpgradeHoldAnnotation] = reason

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
	delete(annotations, RadiusUpgradeHoldAnnotation)
	annotations[RadiusUpgradeCheckedAnnotation] = version

	// Resume the HelmRelease by removing spec.suspend or setting it to false
	spec, found, err := unstructured.NestedMap(hr.Object, "spec")
	if found && err == nil {
		delete(spec, "suspend")
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
