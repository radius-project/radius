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

package kubeutil

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/radius-project/radius/pkg/authz"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

// PodSecurityLevel is the Pod Security Standard Radius applies to the namespaces it deploys to (E1).
const PodSecurityLevel = "baseline"

const podSecurityEnforceLabel = "pod-security.kubernetes.io/enforce"

// PodSecurityLabels returns the Pod Security labels for an environment or application
// namespace in the given mode. dryRun warns and audits; enforce rejects. Off returns nil.
func PodSecurityLabels(mode authz.Mode) map[string]string {
	switch mode {
	case authz.ModeDryRun:
		return map[string]string{
			"pod-security.kubernetes.io/warn":  PodSecurityLevel,
			"pod-security.kubernetes.io/audit": PodSecurityLevel,
		}
	case authz.ModeEnforce:
		return map[string]string{
			"pod-security.kubernetes.io/enforce": PodSecurityLevel,
			"pod-security.kubernetes.io/warn":    PodSecurityLevel,
			"pod-security.kubernetes.io/audit":   PodSecurityLevel,
		}
	}
	return nil
}

// skipPodSecurity lists namespaces Radius never labels.
var skipPodSecurity = map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "radius-system": true}

// ApplyPodSecurityLabels adds the Pod Security labels for the process's authorization
// mode to namespace. It does nothing when the mode is off.
func ApplyPodSecurityLabels(ctx context.Context, client runtime_client.Client, namespace string) error {
	labels := PodSecurityLabels(authz.ProcessMode())
	if labels == nil || namespace == "" || skipPodSecurity[namespace] || namespace == authz.ProcessOptions().Namespace {
		return nil
	}
	patchLabels := map[string]any{}
	for k, v := range labels {
		patchLabels[k] = v
	}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	// When moving from enforce to dryRun, remove the enforce label Radius set earlier.
	if _, ok := labels[podSecurityEnforceLabel]; !ok {
		existing := &corev1.Namespace{}
		if err := client.Get(ctx, runtime_client.ObjectKey{Name: namespace}, existing); err == nil &&
			existing.Labels[podSecurityEnforceLabel] == PodSecurityLevel {
			patchLabels[podSecurityEnforceLabel] = nil
		}
	}
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"labels": patchLabels}})
	if err != nil {
		return err
	}
	if err := client.Patch(ctx, ns, runtime_client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("failed to apply pod security labels to namespace %q: %w", namespace, err)
	}
	return nil
}
