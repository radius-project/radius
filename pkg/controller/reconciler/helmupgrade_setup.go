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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/radius-project/radius/pkg/upgrade/preflight"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/helm"
)

// SetupHelmUpgradeReconciler creates and configures the Helm upgrade reconciler
// with preflight checks and adds it to the controller manager.
func SetupHelmUpgradeReconciler(mgr ctrl.Manager, outputInterface output.Interface, helmInterface helm.Interface) error {
	// Create preflight registry with all the checks
	registry := createPreflightRegistry(outputInterface, helmInterface)

	// Create the reconciler
	reconciler := &HelmUpgradeReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		EventRecorder:     mgr.GetEventRecorderFor("helm-upgrade-controller"),
		PreflightRegistry: registry,
		DelayInterval:     30 * time.Second,
	}

	// Setup the reconciler with the manager
	return reconciler.SetupWithManager(mgr)
}

// createPreflightRegistry creates a preflight registry with all the relevant checks
func createPreflightRegistry(outputInterface output.Interface, helmInterface helm.Interface) *preflight.Registry {
	registry := preflight.NewRegistry(outputInterface)

	// Add standard preflight checks that are relevant for automatic validation
	// Note: We use empty kubeContext as the reconciler runs inside the cluster
	registry.AddCheck(preflight.NewKubernetesConnectivityCheck(""))
	registry.AddCheck(preflight.NewHelmConnectivityCheck(helmInterface, ""))
	registry.AddCheck(preflight.NewRadiusInstallationCheck(helmInterface, ""))
	
	// Add resource availability check (warning only)
	registry.AddCheck(preflight.NewKubernetesResourceCheck(""))

	// Note: Version compatibility and custom config validation checks
	// would need additional information that might not be available in the reconciler context
	// These could be added if the reconciler can determine current/target versions

	return registry
}

// HelmUpgradeEventHandler provides utility methods for handling Helm upgrade events
type HelmUpgradeEventHandler struct {
	EventRecorder record.EventRecorder
}

// RecordUpgradeDetected records an event when a Helm upgrade is detected
func (h *HelmUpgradeEventHandler) RecordUpgradeDetected(obj client.Object, fromVersion, toVersion string, revision int) {
	message := fmt.Sprintf("Detected Radius upgrade from version %s to %s (revision %d)", fromVersion, toVersion, revision)
	h.EventRecorder.Event(obj, corev1.EventTypeNormal, "UpgradeDetected", message)
}

// RecordPreflightSuccess records an event when preflight checks pass
func (h *HelmUpgradeEventHandler) RecordPreflightSuccess(obj client.Object, revision int) {
	message := fmt.Sprintf("Preflight checks passed for Radius revision %d", revision)
	h.EventRecorder.Event(obj, corev1.EventTypeNormal, "PreflightChecksPassed", message)
}

// RecordPreflightFailure records an event when preflight checks fail
func (h *HelmUpgradeEventHandler) RecordPreflightFailure(obj client.Object, revision int, err error) {
	message := fmt.Sprintf("Preflight checks failed for Radius revision %d: %v", revision, err)
	h.EventRecorder.Event(obj, corev1.EventTypeWarning, "PreflightChecksFailed", message)
}