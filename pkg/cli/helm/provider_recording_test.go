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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func providerRecordingFixture(t *testing.T, source, target string) (*providerTransitionStore, providerTransitionPlan, map[string]appsv1.DeploymentSpec, *fake.Clientset) {
	t.Helper()
	store, plan, targets, kube := providerFakeFixture(t, source, target, "enforce")
	plan.Helm = &providerHelmIntent{
		SourceRevision: 7, ChartSHA256: strings.Repeat("a", 64),
		SourceValuesSHA256: strings.Repeat("b", 64), SourceManifestSHA256: strings.Repeat("c", 64),
		TargetValuesSHA256: strings.Repeat("d", 64),
	}
	if target == "external" {
		for _, component := range rotationComponents {
			plan.SourceSecrets = append(plan.SourceSecrets, providerSourceSecret{Name: component + "-mtls", UID: types.UID(component + "-source")})
		}
	}
	generation := strings.ReplaceAll(plan.ID, "-", "")
	for _, base := range providerConfigurationBases[:5] {
		name := base + "-" + generation
		_, err := kube.CoreV1().ConfigMaps(plan.Namespace).Create(t.Context(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: plan.Namespace}, Data: map[string]string{"config": target},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
		pin, err := store.configuration(t.Context(), plan.Namespace, name)
		require.NoError(t, err)
		plan.Configurations = append(plan.Configurations, pin)
		plan.Helm.TargetConfigurations = append(plan.Helm.TargetConfigurations, name)
	}
	require.NoError(t, store.begin(t.Context(), plan, targets, true))
	for range 4 {
		providerStepConverged(t, store, plan, targets)
	}
	_, state, err := store.read(t.Context(), plan.Namespace)
	require.NoError(t, err)
	require.Equal(t, providerRecording, state.Phase)
	return store, plan, targets, kube
}

func TestProviderRecordingRequiresSeparateFencedHandoff(t *testing.T) {
	t.Parallel()
	for _, direction := range [][2]string{{"radius", "external"}, {"external", "radius"}} {
		t.Run(direction[0]+" to "+direction[1], func(t *testing.T) {
			store, plan, targets, _ := providerRecordingFixture(t, direction[0], direction[1])
			require.ErrorContains(t, store.step(t.Context(), plan.Namespace, plan.ID, targets), "pinned final Helm")
			require.ErrorContains(t, store.record(t.Context(), plan.Namespace, plan.ID, targets, nil), "recorder")
			checks := store.checks.(*providerChecksStub)
			before := checks.verified
			require.NoError(t, store.record(t.Context(), plan.Namespace, plan.ID, targets,
				func(ctx context.Context, fence func(context.Context) error) error {
					return fence(ctx)
				}))
			require.Equal(t, before+2, checks.verified)
			_, state, err := store.read(t.Context(), plan.Namespace)
			require.NoError(t, err)
			require.Equal(t, providerComplete, state.Phase)
		})
	}
}

func TestProviderRecordingAllowsOnlyPinnedLifecycleMetadata(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unchanged", "completed CA marker", "different CA marker", "extra annotation", "extra label"} {
		t.Run(scenario, func(t *testing.T) {
			store, plan, targets, kube := providerRecordingFixture(t, "radius", "external")
			deployment, err := kube.AppsV1().Deployments(plan.Namespace).Get(t.Context(), "controller", metav1.GetOptions{})
			require.NoError(t, err)
			switch scenario {
			case "completed CA marker":
				deployment.Annotations[rotationRevision] = plan.Aggregation.Revision
			case "different CA marker":
				deployment.Annotations[rotationRevision] = "different/complete"
			case "extra annotation":
				deployment.Annotations["unrelated"] = "changed"
			case "extra label":
				deployment.Labels["unrelated"] = "changed"
			}
			_, err = kube.AppsV1().Deployments(plan.Namespace).Update(t.Context(), deployment, metav1.UpdateOptions{})
			require.NoError(t, err)
			err = store.record(t.Context(), plan.Namespace, plan.ID, targets,
				func(ctx context.Context, fence func(context.Context) error) error { return fence(ctx) })
			if scenario == "unchanged" || scenario == "completed CA marker" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "metadata drifted")
			}
		})
	}
}

func TestProviderRecordingFailureRetainsRetry(t *testing.T) {
	t.Parallel()
	store, plan, targets, kube := providerRecordingFixture(t, "radius", "external")
	failure := errors.New("Helm final write failed")
	require.ErrorIs(t, store.record(t.Context(), plan.Namespace, plan.ID, targets,
		func(ctx context.Context, fence func(context.Context) error) error {
			require.NoError(t, fence(ctx))
			cm, err := kube.CoreV1().ConfigMaps(plan.Namespace).Get(ctx, plan.Helm.TargetConfigurations[0], metav1.GetOptions{})
			require.NoError(t, err)
			cm.ResourceVersion = "updated-by-helm"
			_, err = kube.CoreV1().ConfigMaps(plan.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
			require.NoError(t, err)
			return failure
		}), failure)
	_, state, err := store.read(t.Context(), plan.Namespace)
	require.NoError(t, err)
	require.Equal(t, providerRecording, state.Phase)
	require.NoError(t, store.record(t.Context(), plan.Namespace, plan.ID, targets,
		func(ctx context.Context, fence func(context.Context) error) error { return fence(ctx) }))
}

func TestProviderRecordingRechecksEvidenceAndWrites(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"target data", "target UID", "journal", "lease", "readiness", "aggregation", "evidence", "cancellation"} {
		t.Run(change, func(t *testing.T) {
			store, plan, targets, kube := providerRecordingFixture(t, "radius", "external")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err := store.record(ctx, plan.Namespace, plan.ID, targets,
				func(ctx context.Context, fence func(context.Context) error) error {
					require.NoError(t, fence(ctx))
					switch change {
					case "target data", "target UID":
						cm, err := kube.CoreV1().ConfigMaps(plan.Namespace).Get(ctx, plan.Helm.TargetConfigurations[0], metav1.GetOptions{})
						require.NoError(t, err)
						if change == "target data" {
							cm.Data["config"] = "off"
						} else {
							cm.UID = types.UID("replacement")
						}
						_, err = kube.CoreV1().ConfigMaps(plan.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
						require.NoError(t, err)
					case "journal":
						cm, _, err := store.read(ctx, plan.Namespace)
						require.NoError(t, err)
						cm.ResourceVersion = "changed"
						_, err = kube.CoreV1().ConfigMaps(plan.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
						require.NoError(t, err)
					case "lease":
						lease, err := kube.CoordinationV1().Leases(plan.Namespace).Get(ctx, providerTransitionLock, metav1.GetOptions{})
						require.NoError(t, err)
						lease.UID = types.UID("replacement")
						_, err = kube.CoordinationV1().Leases(plan.Namespace).Update(ctx, lease, metav1.UpdateOptions{})
						require.NoError(t, err)
					case "readiness":
						d, err := kube.AppsV1().Deployments(plan.Namespace).Get(ctx, "ucp", metav1.GetOptions{})
						require.NoError(t, err)
						d.Status.ReadyReplicas++
						_, err = kube.AppsV1().Deployments(plan.Namespace).UpdateStatus(ctx, d, metav1.UpdateOptions{})
						require.NoError(t, err)
					case "aggregation":
						store.checks.(*providerChecksStub).snapshot.JournalUID = "replacement"
					case "evidence":
						store.checks.(*providerChecksStub).verifyErr = errors.New("receiver restarted")
					case "cancellation":
						cancel()
					}
					return nil
				})
			require.Error(t, err)
			_, state, err := store.read(t.Context(), plan.Namespace)
			require.NoError(t, err)
			require.Equal(t, providerRecording, state.Phase)
		})
	}
}
