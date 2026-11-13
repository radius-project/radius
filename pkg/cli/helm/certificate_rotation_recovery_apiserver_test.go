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
	"crypto/x509"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRotationExpiredTransactionRecovery(t *testing.T) {
	store, _, original := newPlannedRotationTest(t)
	ctx := t.Context()
	advanceRotationTest(t, store, rotationAdopt)
	advanceRotationTest(t, store, rotationIssue)
	staleAPI, err := store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	now := time.Now().Add(366 * 24 * time.Hour)
	store.now = func() time.Time { return now }
	_, _, err = store.read(ctx)
	require.Error(t, err, "ordinary phases cannot publish expired trust")
	observed, _, err := store.readJournal(ctx, true)
	require.NoError(t, err, "expiry must not make incident history unreadable")
	next, _, _ := testSigningCA(t, nil, func(c *x509.Certificate) {
		c.NotBefore, c.NotAfter = now.Add(-time.Hour), now.Add(365*24*time.Hour)
	}, nil, nil)
	digest, _, err := mtls.TrustBundleDigest(next, now)
	require.NoError(t, err)
	id := uuid.NewString()
	plan := *original.Plan
	plan.OldComponentCA = original.NewCA
	plan.TargetTrustMap, plan.TargetTrustSHA256 = "radius-mtls-ca-"+digest[:32], digest
	plan.Issuer, plan.ServingSecret = "radius-ca-"+strings.ReplaceAll(id, "-", "")[:16], "radius-aggregation-"+strings.ReplaceAll(id, "-", "")[:16]
	state, err := store.beginRecovery(ctx, id, next, &plan, observed)
	require.NoError(t, err)
	require.True(t, state.Recovery)
	require.Equal(t, rotationRetire, state.Phase)
	service, err := store.kube.CoreV1().Services(store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	service.Spec.Selector["unexpected"] = "operator-change"
	service, err = store.kube.CoreV1().Services(store.namespace).Update(ctx, service, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, store.reconcileRecoveryRoute(ctx, state.ID), "selector changed outside")
	unchanged, err := store.kube.CoreV1().Services(store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, service.ResourceVersion, unchanged.ResourceVersion)
	require.NoError(t, store.reconcileTrust(ctx, state.ID))
	api, err := store.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	bundle, err := apiServiceBundle(api)
	require.NoError(t, err)
	require.Equal(t, next, bundle, "incident recovery never republishes overlap")
	_, err = store.dynamic.Resource(apiServiceResource).Update(ctx, staleAPI, metav1.UpdateOptions{})
	require.ErrorContains(t, err, "the object has been modified")
	require.Error(t, store.reconcileTrust(ctx, original.ID), "old transactions lose write authority")
	_, err = store.beginRecovery(ctx, uuid.NewString(), next, &plan, observed)
	require.ErrorContains(t, err, "transaction changed", "recovery does not take over a newer journal")
	advanceRotationTest(t, store, rotationComplete)
	now = now.Add(366 * 24 * time.Hour)
	_, completed, err := store.read(ctx)
	require.NoError(t, err, "completed public intent remains inspectable after eventual expiry")
	require.Equal(t, state.ID, completed.ID)
	require.ErrorContains(t, store.reconcileTrust(ctx, state.ID), "no longer valid")
}

func TestRotationLifecycleGuards(t *testing.T) {
	store, _, _ := newPlannedRotationTest(t)
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
	for _, phase := range rotationPhases[1:] {
		advanceRotationTest(t, store, phase)
	}
	require.ErrorContains(t, impl.RollbackRadius(t.Context(), ""), "legacy or unsupported CA journal")
	require.ErrorContains(t, impl.guardCertificateRotation(t.Context(), "", store.namespace, "upgrade",
		&chart.Chart{Metadata: &chart.Metadata{}}), "legacy or unsupported CA journal")
	require.ErrorContains(t, impl.guardCertificateRotation(t.Context(), "", store.namespace, "upgrade",
		&chart.Chart{Metadata: &chart.Metadata{Annotations: map[string]string{"radius.io/ca-rotation-protocol": "1"}}}), "legacy or unsupported CA journal")
	require.NoError(t, store.kube.CoreV1().ConfigMaps(store.namespace).Delete(t.Context(), rotationJournalName, metav1.DeleteOptions{}))
	require.ErrorContains(t, impl.guardCertificateRotation(t.Context(), "", store.namespace, "uninstall", nil), "missing its public journal")
}
