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
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func TestAggregationTransitionSnapshot(t *testing.T) {
	for _, scenario := range []string{"unchanged", "leaf renewal", "APIService replaced", "journal replaced",
		"signer replaced", "issuer replaced", "issuer spec changed", "Certificate replaced", "Certificate spec changed",
		"serving Secret replaced", "projection changed", "trust replaced", "service replaced", "active journal"} {
		t.Run(scenario, func(t *testing.T) {
			s, state := completedAggregationFixture(t)
			ctx, ns := t.Context(), s.namespace
			snapshot, err := s.aggregationTransitionSnapshot(ctx)
			require.NoError(t, err)
			require.True(t, snapshot.Complete)
			require.NotEmpty(t, snapshot.JournalUID)
			require.Equal(t, state.revision(), snapshot.Revision)
			require.Equal(t, state.Plan.TargetTrustSHA256, snapshot.TrustSHA256)
			require.Equal(t, rotationComponentsAndAggregation, snapshot.Scope)
			kube := s.kube.(*kubefake.Clientset)
			dynamic := s.dynamic.(*dynamicfake.FakeDynamicClient)
			kube.ClearActions()
			dynamic.ClearActions()
			replacement := types.UID(uuid.NewString())
			switch scenario {
			case "APIService replaced":
				api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
				require.NoError(t, err)
				api.SetUID(replacement)
				require.NoError(t, dynamic.Tracker().Update(apiServiceResource, api, ""))
			case "journal replaced", "active journal":
				journal, _, err := s.read(ctx)
				require.NoError(t, err)
				if scenario == "journal replaced" {
					journal.UID = replacement
				} else {
					state.Phase = rotationRetire
					data, err := json.Marshal(state)
					require.NoError(t, err)
					journal.Data[rotationJournalKey] = string(data)
				}
				require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("configmaps"), journal, ns))
			case "signer replaced", "serving Secret replaced":
				name := state.Plan.SigningSecret
				if scenario == "serving Secret replaced" {
					name = state.Plan.ServingSecret
				}
				secret, err := s.kube.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
				require.NoError(t, err)
				secret.UID = replacement
				require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), secret, ns))
			case "issuer replaced", "issuer spec changed":
				issuer, err := s.dynamic.Resource(rotationIssuerResource).Namespace(ns).Get(ctx, state.Plan.Issuer, metav1.GetOptions{})
				require.NoError(t, err)
				if scenario == "issuer replaced" {
					issuer.SetUID(replacement)
				} else {
					require.NoError(t, unstructured.SetNestedField(issuer.Object, "changed", "spec", "ca", "crlDistributionPoints"))
				}
				require.NoError(t, dynamic.Tracker().Update(rotationIssuerResource, issuer, ns))
			case "Certificate replaced", "Certificate spec changed", "leaf renewal":
				certificate, err := s.dynamic.Resource(rotationCertificateResource).Namespace(ns).Get(ctx, state.Plan.ServingSecret, metav1.GetOptions{})
				require.NoError(t, err)
				if scenario == "leaf renewal" {
					// The fixture reissues a real same-root leaf with the existing
					// Secret UID when the Certificate is reconciled.
					_, err = s.dynamic.Resource(rotationCertificateResource).Namespace(ns).Update(ctx, certificate, metav1.UpdateOptions{})
					require.NoError(t, err)
				} else {
					if scenario == "Certificate replaced" {
						certificate.SetUID(replacement)
					} else {
						require.NoError(t, unstructured.SetNestedField(certificate.Object, "25h", "spec", "duration"))
					}
					require.NoError(t, dynamic.Tracker().Update(rotationCertificateResource, certificate, ns))
				}
			case "projection changed":
				deployment, err := s.kube.AppsV1().Deployments(ns).Get(ctx, "ucp", metav1.GetOptions{})
				require.NoError(t, err)
				deployment.Spec.Template.Spec.Containers[0].VolumeMounts = nil
				require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), deployment, ns))
			case "trust replaced":
				trust, err := s.kube.CoreV1().ConfigMaps(ns).Get(ctx, state.Plan.TargetTrustMap, metav1.GetOptions{})
				require.NoError(t, err)
				trust.UID = replacement
				require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("configmaps"), trust, ns))
			case "service replaced":
				service, err := s.kube.CoreV1().Services(ns).Get(ctx, "ucp", metav1.GetOptions{})
				require.NoError(t, err)
				service.UID = replacement
				require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("services"), service, ns))
			}
			kube.ClearActions()
			dynamic.ClearActions()
			err = s.checkAggregationTransitionSnapshot(ctx, snapshot)
			if scenario == "unchanged" || scenario == "leaf renewal" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			for _, action := range append(kube.Actions(), dynamic.Actions()...) {
				require.Equal(t, "get", action.GetVerb(), "snapshot/check must never mutate resources")
			}
		})
	}
}

func TestAggregationTransitionSnapshotLegacyAndActive(t *testing.T) {
	runner, _ := rotationRunnerFixture(t)
	s, ctx := runner.store, t.Context()
	_, err := s.aggregationTransitionSnapshot(ctx)
	require.ErrorContains(t, err, "complete the CA rotation")
	require.NoError(t, s.kube.CoreV1().ConfigMaps(s.namespace).Delete(ctx, rotationJournalName, metav1.DeleteOptions{}))
	legacy, err := s.aggregationTransitionSnapshot(ctx)
	require.NoError(t, err)
	require.False(t, legacy.Complete)
	require.Empty(t, legacy.JournalUID)
	require.NotEmpty(t, legacy.APIServiceUID)
	require.Error(t, s.checkAggregationTransitionSnapshot(ctx, legacy))
}

func TestAggregationTransitionSnapshotAllowsQuiescedProviderChange(t *testing.T) {
	s, _ := completedAggregationFixture(t)
	snapshot, err := s.aggregationTransitionSnapshot(t.Context())
	require.NoError(t, err)
	kube := s.kube.(*kubefake.Clientset)
	deployment, err := kube.AppsV1().Deployments(s.namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	deployment.Spec.Replicas = ptr.To(int32(0))
	deployment.Spec.Template.Spec.Containers[0].Env = nil
	require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), deployment, s.namespace))
	require.NoError(t, s.checkAggregationTransitionSnapshot(t.Context(), snapshot),
		"identity adapter is not readiness evidence; provider callback separately verifies final target readiness")
}
