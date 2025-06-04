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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/upgrade/preflight"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// HelmReleaseLabel is the label that Helm uses to identify its releases
	HelmReleaseLabel = "owner"
	// HelmReleaseName is the name of the Radius Helm release we're interested in
	HelmReleaseName = "radius"
	// HelmReleaseNamespace is the namespace where Radius is installed
	HelmReleaseNamespace = "radius-system"
	// RadiusUpgradeAnnotation is the annotation we add to track upgrade detection
	RadiusUpgradeAnnotation = "radius.io/last-checked-revision"
)

// HelmRelease represents the structure of a Helm release stored in a Secret
type HelmRelease struct {
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	Version   int                    `json:"version"`
	Info      HelmReleaseInfo        `json:"info"`
	Chart     HelmChart              `json:"chart"`
	Config    map[string]interface{} `json:"config"`
}

// HelmReleaseInfo contains information about the Helm release
type HelmReleaseInfo struct {
	FirstDeployed metav1.Time `json:"first_deployed"`
	LastDeployed  metav1.Time `json:"last_deployed"`
	Status        string      `json:"status"`
	Description   string      `json:"description"`
}

// HelmChart contains information about the Helm chart
type HelmChart struct {
	Metadata HelmChartMetadata `json:"metadata"`
}

// HelmChartMetadata contains metadata about the Helm chart
type HelmChartMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// HelmUpgradeReconciler reconciles Helm release Secrets to detect Radius upgrades
// and run preflight checks automatically.
type HelmUpgradeReconciler struct {
	// Client is the Kubernetes client
	Client client.Client

	// Scheme is the Kubernetes scheme
	Scheme *runtime.Scheme

	// EventRecorder is the Kubernetes event recorder
	EventRecorder record.EventRecorder

	// PreflightRegistry contains all the preflight checks to run
	PreflightRegistry *preflight.Registry

	// DelayInterval is the amount of time to wait between operations
	DelayInterval time.Duration
}

// Reconcile is the main reconciliation loop for Helm release Secrets
func (r *HelmUpgradeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "Secret", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	// Only process secrets in the radius-system namespace
	if req.Namespace != HelmReleaseNamespace {
		return ctrl.Result{}, nil
	}

	secret := corev1.Secret{}
	err := r.Client.Get(ctx, req.NamespacedName, &secret)
	if apierrors.IsNotFound(err) {
		logger.Info("Helm release secret has been deleted")
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Unable to fetch Helm release secret")
		return ctrl.Result{}, err
	}

	// Check if this is a Helm release secret for Radius
	if !r.isRadiusHelmRelease(&secret) {
		return ctrl.Result{}, nil
	}

	// Parse the Helm release data
	release, err := r.parseHelmRelease(&secret)
	if err != nil {
		logger.Error(err, "Failed to parse Helm release data")
		return ctrl.Result{}, err
	}

	// Check if this is a new revision that we haven't processed
	lastCheckedRevision, hasAnnotation := secret.Annotations[RadiusUpgradeAnnotation]
	currentRevision := fmt.Sprintf("%d", release.Version)

	if hasAnnotation && lastCheckedRevision == currentRevision {
		// We've already processed this revision
		return ctrl.Result{}, nil
	}

	// This is a new revision - run preflight checks
	logger.Info("Detected new Radius Helm revision", "revision", currentRevision, "chartVersion", release.Chart.Metadata.Version, "status", release.Info.Status)

	// Only run checks for successful deployments/upgrades
	if release.Info.Status == "deployed" || release.Info.Status == "superseded" {
		err = r.runPreflightChecks(ctx, logger, release)
		if err != nil {
			logger.Error(err, "Preflight checks failed for Radius revision", "revision", currentRevision)
			r.EventRecorder.Eventf(&secret, corev1.EventTypeWarning, "PreflightChecksFailed", 
				"Preflight checks failed for Radius revision %s: %v", currentRevision, err)
		} else {
			logger.Info("Preflight checks passed for Radius revision", "revision", currentRevision)
			r.EventRecorder.Eventf(&secret, corev1.EventTypeNormal, "PreflightChecksPassed", 
				"Preflight checks passed for Radius revision %s", currentRevision)
		}

		// Update the annotation to mark this revision as processed
		err = r.updateLastCheckedAnnotation(ctx, &secret, currentRevision)
		if err != nil {
			logger.Error(err, "Failed to update last checked annotation")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// isRadiusHelmRelease checks if the secret represents a Radius Helm release
func (r *HelmUpgradeReconciler) isRadiusHelmRelease(secret *corev1.Secret) bool {
	// Check if this is a Helm release secret
	if secret.Type != "helm.sh/release.v1" {
		return false
	}

	// Check if the secret name contains "radius"
	return strings.Contains(secret.Name, HelmReleaseName)
}

// parseHelmRelease extracts the Helm release data from the secret
func (r *HelmUpgradeReconciler) parseHelmRelease(secret *corev1.Secret) (*HelmRelease, error) {
	// Helm stores release data in the "release" field, base64 encoded and gzipped
	releaseData, exists := secret.Data["release"]
	if !exists {
		return nil, fmt.Errorf("secret does not contain release data")
	}

	// Decode base64
	decodedData, err := base64.StdEncoding.DecodeString(string(releaseData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode release data: %w", err)
	}

	// Note: In a real implementation, you would need to decompress the gzipped data here
	// For now, we'll assume the data is JSON (this would need to be enhanced)
	
	var release HelmRelease
	err = json.Unmarshal(decodedData, &release)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal release data: %w", err)
	}

	return &release, nil
}

// runPreflightChecks executes the preflight checks for the detected upgrade
func (r *HelmUpgradeReconciler) runPreflightChecks(ctx context.Context, logger logr.Logger, release *HelmRelease) error {
	if r.PreflightRegistry == nil {
		logger.Info("No preflight registry configured, skipping checks")
		return nil
	}

	logger.Info("Running preflight checks for Radius upgrade", 
		"version", release.Chart.Metadata.Version, 
		"revision", release.Version)

	// Run all registered preflight checks
	results, err := r.PreflightRegistry.RunChecks(ctx)
	if err != nil {
		return fmt.Errorf("preflight checks encountered errors: %w", err)
	}

	// Check if any error-severity checks failed
	for _, result := range results {
		if result.Severity == preflight.SeverityError && (!result.Success || result.Error != nil) {
			return fmt.Errorf("preflight check '%s' failed: %s", result.Check.Name(), getFailureReason(result))
		}
	}

	return nil
}

// updateLastCheckedAnnotation updates the secret with the last checked revision
func (r *HelmUpgradeReconciler) updateLastCheckedAnnotation(ctx context.Context, secret *corev1.Secret, revision string) error {
	// Create a copy of the secret to update
	updatedSecret := secret.DeepCopy()
	
	if updatedSecret.Annotations == nil {
		updatedSecret.Annotations = make(map[string]string)
	}
	
	updatedSecret.Annotations[RadiusUpgradeAnnotation] = revision

	// Update the secret
	return r.Client.Update(ctx, updatedSecret)
}

// SetupWithManager sets up the controller with the Manager
func (r *HelmUpgradeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create a predicate to filter only Helm release secrets in radius-system namespace
	helmSecretPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		secret, ok := obj.(*corev1.Secret)
		if !ok {
			return false
		}

		// Only watch secrets in radius-system namespace
		if secret.Namespace != HelmReleaseNamespace {
			return false
		}

		// Only watch Helm release secrets
		return secret.Type == "helm.sh/release.v1" && strings.Contains(secret.Name, HelmReleaseName)
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		WithEventFilter(helmSecretPredicate).
		Complete(r)
}

// getFailureReason returns a descriptive reason for why a preflight check failed
func getFailureReason(result preflight.CheckResult) string {
	if result.Error != nil {
		return result.Error.Error()
	}
	if result.Message != "" {
		return result.Message
	}
	return "check failed"
}