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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRotationLifecycleGuards(t *testing.T) {
	store, _, _ := newLegacyRotationTest(t)
	impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return store, nil }}
	options := NewDefaultClusterOptions()
	for _, tt := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"install", func(ctx context.Context) error { return impl.InstallRadius(ctx, options, "") }},
		{"upgrade", func(ctx context.Context) error { return impl.UpgradeRadius(ctx, options, "") }},
		{"uninstall", func(ctx context.Context) error { return impl.UninstallRadius(ctx, options, "") }},
		{"rollback", func(ctx context.Context) error { return impl.RollbackRadius(ctx, "") }},
		{"revision rollback", func(ctx context.Context) error { return impl.RollbackRadiusToRevision(ctx, "", 1) }},
		{"preflight", func(ctx context.Context) error { return impl.CheckRadiusCertificates(ctx, options, "") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, tt.run(t.Context()), "legacy or unsupported CA journal", "the nil Helm client must never be reached")
		})
	}
	require.ErrorContains(t, impl.RollbackRadius(t.Context(), ""), "legacy or unsupported CA journal")
	require.ErrorContains(t, impl.guardCertificateRotation(t.Context(), "", store.namespace, "upgrade",
		&chart.Chart{Metadata: &chart.Metadata{}}), "legacy or unsupported CA journal")
	require.ErrorContains(t, impl.guardCertificateRotation(t.Context(), "", store.namespace, "upgrade",
		&chart.Chart{Metadata: &chart.Metadata{Annotations: map[string]string{"radius.io/ca-rotation-protocol": "1"}}}), "legacy or unsupported CA journal")
	require.NoError(t, store.kube.CoreV1().ConfigMaps(store.namespace).Delete(t.Context(), rotationJournalName, metav1.DeleteOptions{}))
	require.ErrorContains(t, impl.guardCertificateRotation(t.Context(), "", store.namespace, "uninstall", nil), "missing its public journal")
}
