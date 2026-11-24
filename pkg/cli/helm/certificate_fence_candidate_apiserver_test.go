//go:build !windows

/*
Copyright 2026 The Radius Authors.

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

package helm

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestCandidateTrustFenceAPIServiceQueuedApply(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for the isolated candidate-policy APIService regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, kube := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	admin, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	services := admin.Resource(candidateAPIResource)
	_, err = kube.RbacV1().ClusterRoles().Create(t.Context(), &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "candidate-injector"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{candidateAPIKind.Group}, Resources: []string{"apiservices"},
			ResourceNames: []string{candidateAPIName}, Verbs: []string{"get", "patch", "update"},
		}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = kube.RbacV1().ClusterRoleBindings().Create(t.Context(), &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "candidate-injector"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "candidate-injector"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "candidate-injector"}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	injectorConfig := rest.CopyConfig(config)
	injectorConfig.Impersonate = rest.ImpersonationConfig{UserName: "candidate-injector", Groups: []string{"system:authenticated"}}
	injector, err := dynamic.NewForConfig(injectorConfig)
	require.NoError(t, err)
	injected := injector.Resource(candidateAPIResource)

	old, _, _ := testSigningCA(t, nil, nil, nil, nil)
	newCA, _, _ := testSigningCA(t, nil, nil, nil, nil)
	overlap := append(append([]byte{}, old...), newCA...)
	_, err = services.Create(t.Context(), candidateTrustService(old, old, nil), metav1.CreateOptions{FieldManager: FieldManager})
	require.NoError(t, err)
	apply := func(bundle []byte, dryRun bool) error {
		t.Helper()
		data, err := json.Marshal(map[string]any{
			"apiVersion": candidateAPIKind.GroupVersion().String(), "kind": candidateAPIKind.Kind,
			"metadata": map[string]any{"name": candidateAPIName},
			"spec":     map[string]any{"caBundle": base64.StdEncoding.EncodeToString(bundle)},
		})
		require.NoError(t, err)
		options := metav1.PatchOptions{FieldManager: "cert-manager-cainjector", Force: ptr.To(true)}
		if dryRun {
			options.DryRun = []string{metav1.DryRunAll}
		}
		_, err = injected.Patch(t.Context(), candidateAPIName, types.ApplyPatchType, data, options)
		return err
	}
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.NoError(c, apply(old, true), "establish actual APIService patch authorization first")
	}, 10*time.Second, 50*time.Millisecond)
	policy, binding := candidateTrustFence()
	_, err = kube.AdmissionregistrationV1().ValidatingAdmissionPolicies().Create(t.Context(), policy, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = kube.AdmissionregistrationV1().ValidatingAdmissionPolicyBindings().Create(t.Context(), binding, metav1.CreateOptions{})
	require.NoError(t, err)

	// This fixture has exactly one private API server with a direct endpoint.
	// This check is not a discovery/activation algorithm for an HA cluster.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		err := apply(overlap, true)
		require.True(c, apierrors.IsForbidden(err), "expected policy rejection: %v", err)
		require.ErrorContains(c, err, candidateFenceName)
	}, 10*time.Second, 50*time.Millisecond)

	update := func(bundle, current, next []byte) {
		t.Helper()
		require.NoError(t, retry.RetryOnConflict(retry.DefaultRetry, func() error {
			live, err := services.Get(t.Context(), candidateAPIName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			desired := candidateTrustService(bundle, current, next)
			live.SetAnnotations(desired.GetAnnotations())
			value, _, err := unstructured.NestedString(desired.Object, "spec", "caBundle")
			if err != nil {
				return err
			}
			if err := unstructured.SetNestedField(live.Object, value, "spec", "caBundle"); err != nil {
				return err
			}
			_, err = services.Update(t.Context(), live, metav1.UpdateOptions{FieldManager: FieldManager})
			return err
		}))
	}
	update(old, old, overlap)
	require.NoError(t, apply(overlap, false))
	update(overlap, overlap, nil)
	require.True(t, apierrors.IsForbidden(apply(old, false)), "queued old-only write must fail after overlap is locked")
	update(newCA, newCA, nil)
	err = apply(overlap, false)
	require.True(t, apierrors.IsForbidden(err), "queued overlap must fail after retirement: %v", err)
	require.ErrorContains(t, err, candidateFenceName)
	require.NoError(t, apply(newCA, false), "current bundle retry remains idempotent")

	live, err := services.Get(t.Context(), candidateAPIName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString(newCA), live.GetAnnotations()[candidateCurrent])
	value, found, err := unstructured.NestedString(live.Object, "spec", "caBundle")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, base64.StdEncoding.EncodeToString(newCA), value)
	forged := candidateTrustService(overlap, overlap, nil)
	forged.SetResourceVersion(live.GetResourceVersion())
	_, err = injected.Update(t.Context(), forged, metav1.UpdateOptions{FieldManager: FieldManager})
	require.True(t, apierrors.IsForbidden(err), "a field-manager name is not coordinator authority: %v", err)

	// Retain the fence until the protected APIService has been removed. This is
	// test cleanup under the fixture administrator, not an uninstall workflow.
	require.NoError(t, services.Delete(t.Context(), candidateAPIName, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: ptr.To(live.GetUID())},
	}))
	require.NoError(t, kube.AdmissionregistrationV1().ValidatingAdmissionPolicyBindings().Delete(t.Context(), binding.Name, metav1.DeleteOptions{}))
	require.NoError(t, kube.AdmissionregistrationV1().ValidatingAdmissionPolicies().Delete(t.Context(), policy.Name, metav1.DeleteOptions{}))
}
