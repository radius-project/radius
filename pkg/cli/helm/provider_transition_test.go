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
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

// This is serialized saved-state input, not simulated execution or observation.
func providerJournalFixture(t *testing.T) *corev1.ConfigMap {
	t.Helper()
	state := providerTransitionState{Version: 2, Phase: providerComplete,
		Plan: providerTransitionPlan{
			ID: uuid.NewString(), Namespace: RadiusSystemNamespace, Release: radiusReleaseName,
			Source: "radius", Target: "external", Mode: "enforce",
			Aggregation: providerAggregationSnapshot{
				Complete: true, APIServiceUID: "api", JournalUID: "aggregation",
				Revision: uuid.NewString() + "/complete", Scope: rotationAggregationOnly,
				TrustSHA256: strings.Repeat("a", 64), SignerSHA256: strings.Repeat("b", 64),
			},
		}}
	for _, name := range []string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"} {
		state.Plan.Workloads = append(state.Plan.Workloads, providerTransitionWorkload{
			Name: name, UID: "workload", ResourceVersion: "1", Replicas: 1,
			SourceSHA256: strings.Repeat("a", 64), TargetSHA256: strings.Repeat("b", 64),
			MetadataSHA256: strings.Repeat("c", 64),
		})
	}
	data, err := json.Marshal(state)
	require.NoError(t, err)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: identityProviderJournal, Namespace: RadiusSystemNamespace,
			UID: "journal", ResourceVersion: "1", Labels: map[string]string{identityProviderMarker: radiusReleaseName}},
		Data: map[string]string{identityProviderStateKey: string(data)},
	}
}

func TestProviderJournalReadOnlyCompatibility(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*corev1.ConfigMap)
		want string
	}{
		{"complete", func(*corev1.ConfigMap) {}, ""},
		{"legacy complete", func(cm *corev1.ConfigMap) { cm.Data[identityProviderStateKey] = `{"version":1,"phase":"complete"}` }, "legacy or unsupported"},
		{"legacy unfinished", func(cm *corev1.ConfigMap) { cm.Data[identityProviderStateKey] = `{"version":1,"phase":"stopping"}` }, "legacy or unsupported"},
		{"malformed", func(cm *corev1.ConfigMap) { cm.Data[identityProviderStateKey] = "{" }, "invalid"},
		{"duplicate escaped phase", func(cm *corev1.ConfigMap) {
			cm.Data[identityProviderStateKey] = strings.Replace(cm.Data[identityProviderStateKey], `"phase":`, `"\u0070hase":"stopping","phase":`, 1)
		}, "invalid"},
		{"trailing JSON", func(cm *corev1.ConfigMap) { cm.Data[identityProviderStateKey] += "{}" }, "trailing"},
		{"unknown fields", func(cm *corev1.ConfigMap) {
			cm.Data[identityProviderStateKey] = strings.Replace(cm.Data[identityProviderStateKey], `"version":`, `"unknown":true,"version":`, 1)
		}, "invalid"},
		{"oversized", func(cm *corev1.ConfigMap) { cm.Data[identityProviderStateKey] = strings.Repeat("x", 65537) }, "invalid"},
		{"wrong owner", func(cm *corev1.ConfigMap) { cm.Labels[identityProviderMarker] = "other" }, "ownership"},
		{"wrong namespace", func(cm *corev1.ConfigMap) {
			cm.Data[identityProviderStateKey] = strings.ReplaceAll(cm.Data[identityProviderStateKey], RadiusSystemNamespace, "other")
		}, "namespace"},
		{"controller substituted", func(cm *corev1.ConfigMap) {
			cm.Data[identityProviderStateKey] = strings.ReplaceAll(cm.Data[identityProviderStateKey], `"controller"`, `"ucp"`)
		}, "workload"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cm := providerJournalFixture(t)
			tt.edit(cm)
			kube := fake.NewClientset(cm)
			store := &rotationStore{kube: kube, namespace: cm.Namespace, release: radiusReleaseName}
			err := (&Impl{}).checkCompletedProviderJournal(t.Context(), store)
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
			for _, action := range kube.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
			retained, err := kube.CoreV1().ConfigMaps(cm.Namespace).Get(t.Context(), cm.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, cm, retained)
		})
	}
	for _, phase := range []string{"stopping", "staging", "starting", "verifying", "recording"} {
		cm := providerJournalFixture(t)
		cm.Data[identityProviderStateKey] = strings.Replace(cm.Data[identityProviderStateKey], `"phase":"complete"`, `"phase":"`+phase+`"`, 1)
		require.Error(t, validateCompletedProviderTransitionJournal(cm, cm.Namespace, radiusReleaseName))
	}
	cm := providerJournalFixture(t)
	var state providerTransitionState
	require.NoError(t, json.Unmarshal([]byte(cm.Data[identityProviderStateKey]), &state))
	state.Plan.Workloads = state.Plan.Workloads[:4]
	require.ErrorContains(t, state.validate(), "all five")
	sentinel := errors.New("additional policy rejected")
	impl := &Impl{validateCompletedProviderJournal: func(*corev1.ConfigMap) error { return sentinel }}
	require.ErrorIs(t, impl.completedProviderJournalValidator(cm.Namespace, radiusReleaseName)(cm), sentinel)
	kube := fake.NewClientset()
	store := &rotationStore{kube: kube, namespace: cm.Namespace}
	require.NoError(t, (&Impl{}).checkCompletedProviderJournal(t.Context(), store))
	kube.PrependReactor("get", "configmaps", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, sentinel
	})
	require.ErrorIs(t, (&Impl{}).checkCompletedProviderJournal(t.Context(), store), sentinel)
}

func providerEntryCalls(i *Impl) map[string]func(context.Context) error {
	options := NewDefaultClusterOptions()
	return map[string]func(context.Context) error{
		"install":           func(ctx context.Context) error { return i.InstallRadius(ctx, options, "") },
		"upgrade":           func(ctx context.Context) error { return i.UpgradeRadius(ctx, options, "") },
		"uninstall":         func(ctx context.Context) error { return i.UninstallRadius(ctx, options, "") },
		"rollback":          func(ctx context.Context) error { return i.RollbackRadius(ctx, "") },
		"rollback revision": func(ctx context.Context) error { return i.RollbackRadiusToRevision(ctx, "", 1) },
	}
}

func TestProviderJournalPinnedSchema(t *testing.T) {
	for _, edit := range []func(*providerTransitionState){
		func(s *providerTransitionState) { s.Plan.Source = "mesh" },
		func(s *providerTransitionState) { s.Plan.Target = s.Plan.Source },
		func(s *providerTransitionState) { s.Plan.Mode = "off" },
		func(s *providerTransitionState) { s.Plan.PreviousJournalUID = "unpaired" },
		func(s *providerTransitionState) { s.Plan.Workloads[0].PreviousMarker = uuid.NewString() + "/starting" },
		func(s *providerTransitionState) { s.Plan.Aggregation.Complete = false },
		func(s *providerTransitionState) { s.Plan.Aggregation.JournalUID = "" },
		func(s *providerTransitionState) { s.Plan.Aggregation.Scope = "" },
		func(s *providerTransitionState) { s.Plan.Aggregation.Revision = uuid.NewString() + "/issue" },
		func(s *providerTransitionState) { s.Plan.Aggregation.SignerSHA256 = strings.Repeat("A", 64) },
		func(s *providerTransitionState) { s.Plan.Workloads[0].ResourceVersion = "" },
		func(s *providerTransitionState) { s.Plan.Workloads[0].UID = "" },
		func(s *providerTransitionState) { s.Plan.Workloads[0].Replicas = -1 },
		func(s *providerTransitionState) { s.Plan.Workloads[0].TargetSHA256 = s.Plan.Workloads[0].SourceSHA256 },
		func(s *providerTransitionState) { s.Plan.Helm.SourceRevision = 0 },
		func(s *providerTransitionState) { s.Plan.Helm.TargetValuesSHA256 = "" },
		func(s *providerTransitionState) { s.Plan.Workloads[0].HelmMetadataSHA256 = "" },
		func(s *providerTransitionState) {
			s.Plan.Helm.TargetConfigurations = s.Plan.Helm.TargetConfigurations[:4]
		},
		func(s *providerTransitionState) { s.Plan.Configurations[0].UID = "" },
		func(s *providerTransitionState) {
			s.Plan.Configurations = append(s.Plan.Configurations, s.Plan.Configurations[0])
		},
	} {
		var state providerTransitionState
		cm := providerJournalFixture(t)
		require.NoError(t, json.Unmarshal([]byte(cm.Data[identityProviderStateKey]), &state))
		hash := strings.Repeat("a", 64)
		state.Plan.Helm = &providerHelmIntent{SourceRevision: 1, ChartSHA256: hash,
			SourceValuesSHA256: hash, SourceManifestSHA256: hash, TargetValuesSHA256: hash}
		for index := range state.Plan.Workloads {
			state.Plan.Workloads[index].HelmMetadataSHA256 = hash
		}
		for _, base := range []string{"ucp-config", "applications-rp-config", "dynamic-rp-config", "controller-config", "bicep-de-config"} {
			name := base + "-" + strings.ReplaceAll(state.Plan.ID, "-", "")
			state.Plan.Helm.TargetConfigurations = append(state.Plan.Helm.TargetConfigurations, name)
			state.Plan.Configurations = append(state.Plan.Configurations, providerTransitionConfig{
				Name: name, UID: "configuration", ResourceVersion: "1", SHA256: hash,
			})
		}
		require.NoError(t, state.validate(), "completed saved Helm intent remains readable")
		edit(&state)
		require.Error(t, state.validate())
	}
}

func TestProviderJournalPublicEntriesBeforeWrites(t *testing.T) {
	for _, raw := range []string{`{"version":1,"phase":"complete"}`, `{"version":2,"phase":"complete"}`, "{"} {
		cm := providerJournalFixture(t)
		cm.Data[identityProviderStateKey] = raw
		kube := fake.NewClientset(cm)
		store := &rotationStore{kube: kube, namespace: cm.Namespace, release: radiusReleaseName}
		impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return store, nil }}
		for name, call := range providerEntryCalls(impl) {
			t.Run(name+"/"+raw, func(t *testing.T) { require.Error(t, call(t.Context())) })
		}
		for _, action := range kube.Actions() {
			require.Equal(t, "get", action.GetVerb())
		}
	}
	// Ordinary operation guards accepted full completed-v2 input before this
	// reduction. Keep that read-only behavior rather than rejecting every journal.
	cm := providerJournalFixture(t)
	kube := fake.NewClientset(cm)
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	store := &rotationStore{kube: kube, dynamic: dynamic, namespace: cm.Namespace, release: radiusReleaseName}
	impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return store, nil }}
	for _, operation := range []string{"install", "upgrade", "uninstall", "rollback"} {
		require.NoError(t, impl.guardCertificateRotation(t.Context(), "", cm.Namespace, operation, nil))
	}
	for _, actions := range [][]ktesting.Action{kube.Actions(), dynamic.Actions()} {
		for _, action := range actions {
			require.Contains(t, []string{"get", "list"}, action.GetVerb())
		}
	}
}
