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
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestApplyPodSecurityLabels(t *testing.T) {
	t.Cleanup(func() { authz.SetProcessOptions(authz.Options{}) })

	cases := []struct {
		mode authz.Mode
		want map[string]string
	}{
		{authz.ModeOff, nil},
		{authz.ModeDryRun, map[string]string{"pod-security.kubernetes.io/warn": "baseline", "pod-security.kubernetes.io/audit": "baseline"}},
		{authz.ModeEnforce, map[string]string{"pod-security.kubernetes.io/enforce": "baseline", "pod-security.kubernetes.io/warn": "baseline", "pod-security.kubernetes.io/audit": "baseline"}},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			authz.SetProcessOptions(authz.Options{Mode: tc.mode})
			c := fake.NewClientBuilder().WithObjects(
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "radius-system"}},
			).Build()

			require.NoError(t, ApplyPodSecurityLabels(t.Context(), c, "app"))
			require.NoError(t, ApplyPodSecurityLabels(t.Context(), c, "radius-system"))

			ns := &corev1.Namespace{}
			require.NoError(t, c.Get(t.Context(), client.ObjectKey{Name: "app"}, ns))
			if tc.want == nil {
				require.Empty(t, ns.Labels)
			} else {
				require.Equal(t, tc.want, ns.Labels)
			}
			require.NoError(t, c.Get(t.Context(), client.ObjectKey{Name: "radius-system"}, ns))
			require.Empty(t, ns.Labels)
		})
	}
}

func TestApplyPodSecurityLabels_DryRunRemovesEnforce(t *testing.T) {
	t.Cleanup(func() { authz.SetProcessOptions(authz.Options{}) })
	authz.SetProcessOptions(authz.Options{Mode: authz.ModeDryRun})

	c := fake.NewClientBuilder().WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ours", Labels: map[string]string{"pod-security.kubernetes.io/enforce": "baseline"}}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "user", Labels: map[string]string{"pod-security.kubernetes.io/enforce": "restricted"}}},
	).Build()
	require.NoError(t, ApplyPodSecurityLabels(t.Context(), c, "ours"))
	require.NoError(t, ApplyPodSecurityLabels(t.Context(), c, "user"))

	ns := &corev1.Namespace{}
	require.NoError(t, c.Get(t.Context(), client.ObjectKey{Name: "ours"}, ns))
	require.NotContains(t, ns.Labels, "pod-security.kubernetes.io/enforce")
	require.Equal(t, "baseline", ns.Labels["pod-security.kubernetes.io/warn"])
	require.NoError(t, c.Get(t.Context(), client.ObjectKey{Name: "user"}, ns))
	require.Equal(t, "restricted", ns.Labels["pod-security.kubernetes.io/enforce"])
}
