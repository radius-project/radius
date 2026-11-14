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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func pinInjectorFixture(t *testing.T, w *certificateExecutionWriter) {
	t.Helper()
	deployment, replicaSet, pod := injectorFixture("cert-manager-cainjector")
	for _, object := range []runtime.Object{deployment, replicaSet, pod} {
		require.NoError(t, w.store.kube.(*kubefake.Clientset).Tracker().Add(object))
	}

	instances, err := inspectCAInjectorPrerequisites(t.Context(), w.store.kube, w.store.namespace)
	require.NoError(t, err)
	w.state.Execution.CAInjectorSHA256, err = bootstrapDigest(instances)
	require.NoError(t, err)
}

func TestCertificateSelectedPhaseRendering(t *testing.T) {
	w := lifecycleExecutionFixture(t)
	selected, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	values := map[string]any{"global": map[string]any{"rbac": map[string]any{
		"enabled": true, "tls": map[string]any{"provider": "radius"},
		"certificates": map[string]any{"mode": "selfSigned"},
	}}}
	target, err := certificateProfilePlan(selected, values, w.store.namespace)
	require.NoError(t, err)
	w.state.Execution.ChartSHA256, err = bootstrapDigest(selected)
	require.NoError(t, err)
	w.state.Execution.BaselineValuesSHA256, err = bootstrapDigest(values)
	require.NoError(t, err)
	w.state.Execution.TargetValuesSHA256, err = bootstrapDigest(target.Values)
	require.NoError(t, err)
	w.state.Execution.ServingSecret = "radius-aggregation-" + w.state.ID
	w.state.Execution.PreviousServingSecret = "ucp-apiserver-tls"
	options := NewDefaultClusterOptions()
	options.RadiusInstalled, options.RadiusReleaseValues = true, copyValues(values)
	history := []*releasev1.Release{{Name: w.store.release, Namespace: w.store.namespace, Version: 1,
		Chart: selected, Config: copyValues(values), Info: &releasev1.Info{Status: releasecommon.StatusDeployed}}}
	impl := &Impl{contourCluster: &fakeContourCluster{}}
	// This uses the real selected templates and in-memory Helm history. It is
	// not controller issuance, dry-run API admission, or authenticated evidence.
	w.observeLoaded = func(ctx context.Context, _ certificateLifecycle) error { return ctx.Err() }
	require.NoError(t, withIdentityOperation(t.Context(), w.store.kube, w.store.namespace, func(operation *identityOperation) error {
		w.operation = operation
		require.NoError(t, w.save(t.Context(), w.state))
		for _, name := range []string{"internal", "aggregation"} {
			require.NoError(t, w.publishSource(t.Context(), name, w.state.ID+"/publish"))
		}
		convergeAPIServiceFixture(t, w)
		executor := &certificateLifecycleExecutor{writer: w}
		require.NoError(t, executor.advance(t.Context(), rotationPublish))
		for _, phase := range []rotationPhase{rotationAdopt, rotationIssue} {
			require.NoError(t, executor.advance(t.Context(), phase))
			objects, err := w.renderSelectedPhase(t.Context(), impl, options, selected, target, history, "")
			require.NoError(t, err)
			foundLeaf, foundUCP := false, false
			for _, object := range objects {
				if object.GetKind() == "Certificate" {
					foundLeaf = true
					name, _, err := unstructured.NestedString(object.Object, "spec", "secretName")
					require.NoError(t, err)
					require.Equal(t, w.state.Execution.ServingSecret, name)
					issuer, _, err := unstructured.NestedString(object.Object, "spec", "issuerRef", "name")
					require.NoError(t, err)
					require.Equal(t, w.state.Planes["aggregation"].Issuer.Name, issuer)
				} else if object.GetName() == "ucp" {
					foundUCP = true
					nonce, _, err := unstructured.NestedString(object.Object, "spec", "template", "metadata", "annotations", certificateExecutionOwner)
					require.NoError(t, err)
					if phase == rotationIssue {
						require.Equal(t, w.state.ID+"/issue", nonce)
					} else {
						require.Empty(t, nonce)
					}
				}
			}
			require.Equal(t, phase == rotationIssue, foundLeaf)
			require.True(t, foundUCP)
			hash, err := bootstrapDigest(selected)
			require.NoError(t, err)
			require.Equal(t, w.state.Execution.ChartSHA256, hash, "render projection must not mutate the chart persisted in Helm history")
		}
		return nil
	}))
}
func TestCertificateLifecyclePhaseExecutorFixtures(t *testing.T) {
	for _, scenario := range []string{"complete", "observer unavailable", "interrupted final reconciliation"} {
		t.Run(scenario, func(t *testing.T) {
			w := lifecycleExecutionFixture(t)
			pinInjectorFixture(t, w)
			target := &certificateTarget{Mode: w.state.Mode, Provider: w.state.Provider, Values: map[string]any{}}
			var err error
			w.state.Execution.TargetValuesSHA256, err = bootstrapDigest(target.Values)
			require.NoError(t, err)
			executor := &certificateLifecycleExecutor{writer: w, target: target, options: NewDefaultClusterOptions()}
			executor.options.Radius.Timeout = 2 * time.Second
			kube := w.store.kube.(*kubefake.Clientset)
			if scenario == "observer unavailable" {
				kube.ClearActions()
				require.ErrorIs(t, executor.run(t.Context(), rotationPublish), errCertificateObserverUnavailable)
				require.Empty(t, kube.Actions(), "missing observation coverage/access fails before Lease or any other API mutation")
				return
			}
			// These are explicit orchestration fixtures, not real authenticated
			// observers, workload rollouts, Helm execution, or product acceptance.
			w.observeLoaded = func(ctx context.Context, _ certificateLifecycle) error { return ctx.Err() }
			w.observeServing = func(ctx context.Context, _ certificateLifecycle) error { return ctx.Err() }
			applied := map[rotationPhase]int{}
			executor.applyPhaseFixture = func(ctx context.Context) error {
				applied[w.state.Phase]++
				return w.check(ctx)
			}
			commitCalls := 0
			executor.commitFixture = func(ctx context.Context) (int, error) {
				commitCalls++
				if scenario == "interrupted final reconciliation" && commitCalls == 1 {
					return 0, errors.New("fixture interruption during final Helm reconciliation")
				}
				return w.state.Execution.BaselineVersion + 1, ctx.Err()
			}
			for _, phase := range []rotationPhase{rotationPublish, rotationAdopt, rotationIssue} {
				require.NoError(t, executor.run(t.Context(), phase))
				require.NoError(t, executor.run(t.Context(), phase), "completed phase retry is idempotent")
				resumeExecutionFixture(t, w)
			}
			require.Equal(t, rotationReady, w.state.Phase)
			for _, plane := range w.state.Planes {
				require.NotEqual(t, plane.NewCA, plane.Source.Bundle, "issuance never implicitly retires old trust")
			}
			err = executor.run(t.Context(), rotationRetire)
			if scenario == "interrupted final reconciliation" {
				require.ErrorContains(t, err, "fixture interruption")
				require.Equal(t, rotationComplete, w.state.Phase)
				require.Zero(t, w.state.Execution.FinalReleaseVersion)
				_, err := w.store.aggregationTransitionSnapshot(t.Context())
				require.ErrorContains(t, err, "complete the CA rotation")
				resumeExecutionFixture(t, w)
				require.NoError(t, executor.run(t.Context(), rotationRetire))
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, rotationComplete, w.state.Phase)
			require.Equal(t, 2, w.state.Execution.FinalReleaseVersion)
			require.Equal(t, 1, applied[rotationAdopt])
			require.Equal(t, 1, applied[rotationIssue])
			for _, plane := range w.state.Planes {
				require.Equal(t, plane.NewCA, plane.Source.Bundle)
			}
			require.NoError(t, w.checkAggregationTrust(t.Context()))
			before := commitCalls
			require.NoError(t, executor.run(t.Context(), rotationRetire))
			require.Equal(t, before, commitCalls)
		})
	}
}
