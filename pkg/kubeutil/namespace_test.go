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
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/retry"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func Test_DeploymentTargetAuthorization(t *testing.T) {
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		for _, tt := range []struct {
			name         string
			targetUID    types.UID
			missingCP    bool
			missing      bool
			wantError    string
			wantDistinct bool
		}{
			{name: "different cluster same namespace name", targetUID: "external", wantDistinct: true},
			{name: "same cluster through another client", targetUID: "control-plane"},
			{name: "empty UID", wantError: "UID is empty"},
			{name: "target identity read fails", missing: true, wantError: "failed to identify deployment-target cluster"},
			{name: "control-plane identity read fails", missingCP: true, targetUID: "external", wantError: "failed to identify control-plane cluster"},
		} {
			t.Run(string(mode)+"/"+tt.name, func(t *testing.T) {
				t.Setenv(TargetKubeconfigEnvVar, "configured-target")
				controlPlane := k8sutil.NewFakeKubeClient(nil)
				if !tt.missingCP {
					require.NoError(t, controlPlane.Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: "control-plane"}}))
				}
				target := k8sutil.NewFakeKubeClient(nil)
				if !tt.missing {
					require.NoError(t, target.Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: tt.targetUID}}))
				}
				opts := authz.Options{Mode: mode}
				got, err := DeploymentTargetAuthorization(t.Context(), opts, controlPlane, target, "radius-system")
				if mode != authz.ModeOff && tt.wantError != "" {
					require.ErrorContains(t, err, tt.wantError)
					require.False(t, got.TargetClusterIsDistinct)
					if tt.missing || tt.missingCP {
						require.True(t, apierrors.IsNotFound(err))
					}
					return
				}
				require.NoError(t, err)
				require.Equal(t, mode != authz.ModeOff && tt.wantDistinct, got.TargetClusterIsDistinct)
				require.False(t, opts.TargetClusterIsDistinct, "request-scoped resolution must not alter shared configuration")

				labels := got.PodSecurityLabels("radius-system")
				if mode != authz.ModeOff && tt.wantDistinct {
					require.Equal(t, "baseline", labels[authz.PodSecurityLabelWarn])
					require.Equal(t, "baseline", labels[authz.PodSecurityLabelAudit])
					require.Equal(t, mode == authz.ModeEnforce, labels[authz.PodSecurityLabelEnforce] == "baseline")
					require.Nil(t, got.PodSecurityLabels("kube-system"))
				} else {
					require.Nil(t, labels)
				}
			})
		}
	}

	t.Run("ordinary namespace needs no identity read", func(t *testing.T) {
		t.Setenv(TargetKubeconfigEnvVar, "configured-target")
		opts := authz.Options{Mode: authz.ModeEnforce}
		got, err := DeploymentTargetAuthorization(t.Context(), opts, nil, nil, "application")
		require.NoError(t, err)
		require.Equal(t, opts, got)
	})
	t.Run("single-cluster needs no identity read", func(t *testing.T) {
		t.Setenv(TargetKubeconfigEnvVar, "")
		opts := authz.Options{Mode: authz.ModeEnforce}
		got, err := DeploymentTargetAuthorization(t.Context(), opts, nil, nil, "radius-system")
		require.NoError(t, err)
		require.Equal(t, opts, got)
	})
	t.Run("missing clients cannot establish separation", func(t *testing.T) {
		t.Setenv(TargetKubeconfigEnvVar, "configured-target")
		got, err := DeploymentTargetAuthorization(t.Context(), authz.Options{Mode: authz.ModeEnforce}, nil, nil, "radius-system")
		require.ErrorContains(t, err, "without both Kubernetes clients")
		require.False(t, got.TargetClusterIsDistinct)
	})
}

func Test_PatchNamespace(t *testing.T) {
	client := k8sutil.NewFakeKubeClient(scheme.Scheme)

	err := PatchNamespace(t.Context(), client, "test")
	require.NoError(t, err)

	ns := &unstructured.Unstructured{}
	ns.SetAPIVersion("v1")
	ns.SetKind("Namespace")

	err = client.Get(t.Context(), runtime_client.ObjectKey{Name: "test"}, ns)
	require.NoError(t, err)

	expected := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name":            "test",
				"resourceVersion": "1",
				"labels": map[string]any{
					"app.kubernetes.io/managed-by": "radius-rp",
				},
			},
		},
	}
	require.Equal(t, expected, ns)
}

func Test_AddMissingNamespaceLabels_ConcurrentChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		concurrent  map[string]string
		wantAdded   map[string]string
		wantLabels  map[string]string
		wantPatches int
	}{
		{
			name:        "retries an unrelated change",
			concurrent:  map[string]string{"team": "ops"},
			wantAdded:   map[string]string{authz.PodSecurityLabelWarn: "baseline", authz.PodSecurityLabelAudit: "baseline"},
			wantLabels:  map[string]string{"team": "ops", authz.PodSecurityLabelWarn: "baseline", authz.PodSecurityLabelAudit: "baseline"},
			wantPatches: 2,
		},
		{
			name:        "preserves a concurrent operator override",
			concurrent:  map[string]string{authz.PodSecurityLabelWarn: "privileged"},
			wantAdded:   map[string]string{authz.PodSecurityLabelAudit: "baseline"},
			wantLabels:  map[string]string{authz.PodSecurityLabelWarn: "privileged", authz.PodSecurityLabelAudit: "baseline"},
			wantPatches: 2,
		},
		{
			name:        "does not patch labels another writer supplied",
			concurrent:  map[string]string{authz.PodSecurityLabelWarn: "restricted", authz.PodSecurityLabelAudit: "restricted"},
			wantLabels:  map[string]string{authz.PodSecurityLabelWarn: "restricted", authz.PodSecurityLabelAudit: "restricted"},
			wantPatches: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			patches := 0
			client := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).
				WithObjects(&corev1.Namespace{Name: "apps"}).
				WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(ctx context.Context, client runtime_client.WithWatch, obj runtime_client.Object, patch runtime_client.Patch, opts ...runtime_client.PatchOption) error {
						patches++
						if patches == 1 {
							current := &corev1.Namespace{}
							require.NoError(t, client.Get(ctx, runtime_client.ObjectKey{Name: obj.GetName()}, current))
							current.Labels = tt.concurrent
							require.NoError(t, client.Update(ctx, current))
						}
						return client.Patch(ctx, obj, patch, opts...)
					},
				}).Build()

			added, err := AddMissingNamespaceLabels(t.Context(), client, &corev1.Namespace{Name: "apps"}, map[string]string{
				authz.PodSecurityLabelWarn: "baseline", authz.PodSecurityLabelAudit: "baseline",
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantAdded, added)
			require.Equal(t, tt.wantPatches, patches)
			ns := &corev1.Namespace{}
			require.NoError(t, client.Get(t.Context(), runtime_client.ObjectKey{Name: "apps"}, ns))
			require.Equal(t, tt.wantLabels, ns.Labels)
		})
	}
}

func Test_AddMissingNamespaceLabels_Errors(t *testing.T) {
	t.Parallel()

	conflict := apierrors.NewConflict(schema.GroupResource{Resource: "namespaces"}, "apps", errors.New("concurrent update"))
	for _, patchErr := range []error{conflict, errors.New("patch denied")} {
		t.Run(patchErr.Error(), func(t *testing.T) {
			t.Parallel()

			patches := 0
			client := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).
				WithObjects(&corev1.Namespace{Name: "apps"}).
				WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(context.Context, runtime_client.WithWatch, runtime_client.Object, runtime_client.Patch, ...runtime_client.PatchOption) error {
						patches++
						return patchErr
					},
				}).Build()

			added, err := AddMissingNamespaceLabels(t.Context(), client, &corev1.Namespace{Name: "apps"}, map[string]string{authz.PodSecurityLabelWarn: "baseline"})
			require.ErrorIs(t, err, patchErr)
			require.Nil(t, added)
			if apierrors.IsConflict(patchErr) {
				require.Equal(t, retry.DefaultRetry.Steps, patches)
			} else {
				require.Equal(t, 1, patches)
			}
			ns := &corev1.Namespace{}
			require.NoError(t, client.Get(t.Context(), runtime_client.ObjectKey{Name: "apps"}, ns))
			require.Empty(t, ns.Labels)
		})
	}

	t.Run("namespace read failure", func(t *testing.T) {
		t.Parallel()
		client := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
		added, err := AddMissingNamespaceLabels(t.Context(), client, &corev1.Namespace{Name: "missing"}, map[string]string{authz.PodSecurityLabelWarn: "baseline"})
		require.True(t, apierrors.IsNotFound(err))
		require.Nil(t, added)
	})
}

func Test_CreateNamespace_PodSecurityLabels(t *testing.T) {
	t.Parallel()

	baselineWarnAudit := map[string]string{authz.PodSecurityLabelWarn: "baseline", authz.PodSecurityLabelAudit: "baseline"}
	operatorLabels := map[string]string{"team": "a", authz.PodSecurityLabelWarn: "privileged"}

	tests := []struct {
		name        string
		opts        authz.Options
		namespace   string
		existing    map[string]string // nil means the namespace does not exist yet
		wantCreated bool
		wantAdded   map[string]string
		wantLabels  map[string]string
	}{
		{name: "off creates without labels", opts: authz.Options{}, namespace: "apps", wantCreated: true},
		{name: "off leaves existing namespace alone", opts: authz.Options{Mode: authz.ModeOff}, namespace: "apps", existing: map[string]string{"team": "a"}, wantLabels: map[string]string{"team": "a"}},
		{name: "dryRun creates with warn and audit", opts: authz.Options{Mode: authz.ModeDryRun}, namespace: "apps", wantCreated: true, wantAdded: baselineWarnAudit, wantLabels: baselineWarnAudit},
		{
			name: "enforce restricted creates with all labels", opts: authz.Options{Mode: authz.ModeEnforce, PodSecurityLevel: authz.PodSecurityLevelRestricted}, namespace: "apps", wantCreated: true,
			wantAdded:  map[string]string{authz.PodSecurityLabelEnforce: "restricted", authz.PodSecurityLabelWarn: "restricted", authz.PodSecurityLabelAudit: "restricted"},
			wantLabels: map[string]string{authz.PodSecurityLabelEnforce: "restricted", authz.PodSecurityLabelWarn: "restricted", authz.PodSecurityLabelAudit: "restricted"},
		},
		{name: "dryRun labels existing namespace", opts: authz.Options{Mode: authz.ModeDryRun}, namespace: "apps", existing: map[string]string{}, wantAdded: baselineWarnAudit, wantLabels: baselineWarnAudit},
		{
			name: "enforce keeps operator labels", opts: authz.Options{Mode: authz.ModeEnforce}, namespace: "apps", existing: operatorLabels,
			wantAdded:  map[string]string{authz.PodSecurityLabelEnforce: "baseline", authz.PodSecurityLabelAudit: "baseline"},
			wantLabels: map[string]string{"team": "a", authz.PodSecurityLabelWarn: "privileged", authz.PodSecurityLabelEnforce: "baseline", authz.PodSecurityLabelAudit: "baseline"},
		},
		{
			name: "level change preserves existing labels", opts: authz.Options{Mode: authz.ModeDryRun, PodSecurityLevel: authz.PodSecurityLevelRestricted}, namespace: "apps", existing: baselineWarnAudit,
			wantLabels: baselineWarnAudit,
		},
		{
			name: "enabling enforce adds only the missing label", opts: authz.Options{Mode: authz.ModeEnforce, PodSecurityLevel: authz.PodSecurityLevelRestricted}, namespace: "apps", existing: baselineWarnAudit,
			wantAdded:  map[string]string{authz.PodSecurityLabelEnforce: "restricted"},
			wantLabels: map[string]string{authz.PodSecurityLabelWarn: "baseline", authz.PodSecurityLabelAudit: "baseline", authz.PodSecurityLabelEnforce: "restricted"},
		},
		{
			name: "off preserves existing enforcement", opts: authz.Options{Mode: authz.ModeOff}, namespace: "apps",
			existing:   map[string]string{authz.PodSecurityLabelEnforce: "baseline"},
			wantLabels: map[string]string{authz.PodSecurityLabelEnforce: "baseline"},
		},
		{name: "enforce skips control-plane namespace", opts: authz.Options{Mode: authz.ModeEnforce}, namespace: "radius-system", existing: map[string]string{}, wantLabels: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var objs []runtime_client.Object
			if tt.existing != nil {
				objs = append(objs, &corev1.Namespace{Name: tt.namespace, Labels: tt.existing})
			}
			client := k8sutil.NewFakeKubeClient(clientgoscheme.Scheme, objs...)

			created, added, err := CreateNamespace(t.Context(), client, tt.namespace, tt.opts)
			require.NoError(t, err)
			require.Equal(t, tt.wantCreated, created)
			require.Equal(t, tt.wantAdded, added)

			ns := &corev1.Namespace{}
			require.NoError(t, client.Get(t.Context(), runtime_client.ObjectKey{Name: tt.namespace}, ns))
			if len(tt.wantLabels) == 0 {
				require.Empty(t, ns.Labels)
			} else {
				require.Equal(t, tt.wantLabels, ns.Labels)
			}
		})
	}
}
