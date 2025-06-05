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

	// Run preflight checks
	logger.Info("Running preflight checks for Radius upgrade", "version", chartVersion)
	
	if err := r.runPreflightChecks(ctx); err != nil {
		r.EventRecorder.Event(helmRelease, "Warning", "PreflightFailed", 
			fmt.Sprintf("Preflight checks failed: %v", err))
		return ctrl.Result{RequeueAfter: time.Minute * 5}, err
	}

	// Mark as processed
	if err := r.markPreflightComplete(ctx, helmRelease, chartVersion); err != nil {
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

// getNestedString safely extracts a nested string from unstructured data
func (r *FluxHelmReleaseReconciler) getNestedString(hr *unstructured.Unstructured, fields ...string) string {
	value, found, err := unstructured.NestedString(hr.Object, fields...)
	if !found || err != nil {
		return ""
	}
	return value
}

// runPreflightChecks executes all registered preflight checks
func (r *FluxHelmReleaseReconciler) runPreflightChecks(ctx context.Context) error {
	if r.PreflightRegistry == nil {
		return nil // No checks configured
	}

	// Run all checks - the registry handles the execution
	_, err := r.PreflightRegistry.RunChecks(ctx)
	return err
}

// markPreflightComplete adds annotation to track preflight completion
func (r *FluxHelmReleaseReconciler) markPreflightComplete(ctx context.Context, hr *unstructured.Unstructured, version string) error {
	annotations := hr.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[RadiusPreflightAnnotation] = version
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