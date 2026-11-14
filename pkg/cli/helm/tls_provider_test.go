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
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"
)

func TestTLSProviderFlags(t *testing.T) {
	for _, tt := range []struct {
		args []string
		fail bool
	}{
		{nil, false}, {[]string{"--tls-provider=radius"}, false}, {[]string{"--tls-provider=external"}, false},
		{[]string{"--tls-provider="}, true}, {[]string{"--tls-provider=managed"}, true},
		{[]string{"--mtls-provider=external"}, true}, {[]string{"--mtls-provider="}, true},
		{[]string{"--mtls-provider=radius", "--tls-provider=radius"}, true},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var options TLSProviderOptions
			flags := pflag.NewFlagSet("provider", pflag.ContinueOnError)
			options.AddFlags(flags)
			require.NoError(t, flags.Parse(tt.args))
			err := options.ValidateFlags(flags)
			if tt.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func providerTestValues(provider string) map[string]any {
	return map[string]any{"global": map[string]any{"rbac": map[string]any{
		"enabled": true, "tls": map[string]any{"provider": provider},
	}}}
}

func TestTLSProviderSelection(t *testing.T) {
	defaults := providerTestValues("radius")
	for _, tt := range []struct {
		name    string
		options ClusterOptions
		current map[string]any
		fail    string
	}{
		{name: "fresh external", options: ClusterOptions{TLS: TLSProviderOptions{Provider: "external"}}, current: map[string]any{}},
		{name: "preserve external", options: ClusterOptions{RadiusReleaseValues: providerTestValues("external")}, current: map[string]any{}},
		{name: "explicit same provider", options: ClusterOptions{RadiusReleaseValues: providerTestValues("external"), TLS: TLSProviderOptions{Provider: "external"}}, current: map[string]any{}},
		{name: "switch radius external", options: ClusterOptions{RadiusReleaseValues: providerTestValues("radius"), TLS: TLSProviderOptions{Provider: "external"}}, current: map[string]any{}, fail: "blocked before mutation"},
		{name: "switch external radius", options: ClusterOptions{RadiusReleaseValues: providerTestValues("external")}, current: providerTestValues("radius"), fail: "blocked before mutation"},
		{name: "dual selector", options: ClusterOptions{TLS: TLSProviderOptions{Provider: "external"}}, current: providerTestValues("external"), fail: "cannot be combined"},
		{name: "reset stored", options: ClusterOptions{RadiusReleaseValues: providerTestValues("external"), ResetValues: true}, current: map[string]any{}, fail: "cannot discard"},
		{name: "scalar off cannot discard external", options: ClusterOptions{RadiusReleaseValues: providerTestValues("external")},
			current: map[string]any{"global": map[string]any{"rbac": false}}, fail: "blocked before mutation"},
		{name: "legacy value", current: map[string]any{"global": map[string]any{"rbac": map[string]any{"mtls": map[string]any{"provider": "external"}}}}, fail: "must not default"},
		{name: "legacy stored", options: ClusterOptions{RadiusReleaseValues: map[string]any{"global": map[string]any{"rbac": map[string]any{"mtls": map[string]any{"provider": "external"}}}}}, current: map[string]any{}, fail: "must not default"},
		{name: "typo", current: map[string]any{"global": map[string]any{"rbac": map[string]any{"tlsProvider": "external"}}}, fail: "not global.rbac.tlsProvider"},
		{name: "malformed mapping", current: map[string]any{"global": "invalid"}, fail: "invalid"},
		{name: "off", options: ClusterOptions{TLS: TLSProviderOptions{Provider: "radius"}}, current: map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": false}}}, fail: "requires active"},
		{name: "old engine gate", current: map[string]any{"de": map[string]any{"externalIdentity": map[string]any{"enabled": true}}}, fail: "internalAuth"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := copyValues(tt.options.RadiusReleaseValues)
			var logs []string
			impl := &Impl{logger: func(message string, _ ...any) { logs = append(logs, message) }}
			err := impl.prepareTLSProvider(tt.options, defaults, tt.current, tt.options.RadiusReleaseValues != nil)
			if tt.options.RadiusReleaseValues != nil {
				require.Equal(t, before, tt.options.RadiusReleaseValues)
			}
			if tt.fail != "" {
				require.ErrorContains(t, err, tt.fail)
				return
			}
			require.NoError(t, err)
			require.Contains(t, strings.Join(logs, " "), "EVERY connection")
			if tt.options.TLS.Provider != "" {
				provider, found, err := providerValue(tt.current)
				require.NoError(t, err)
				require.True(t, found)
				require.Equal(t, tt.options.TLS.Provider, provider)
			}
		})
	}
}

func TestProviderMissingObservationBeforeLease(t *testing.T) {
	client := fake.NewClientset()
	store := &providerTransitionStore{kube: client}
	require.ErrorIs(t, store.withLock(t.Context(), "radius-system", func(*coordinationv1.Lease) error { return nil }), errProviderObservationUnavailable)
	require.Empty(t, client.Actions())
}

func TestProviderLegacyJournalAndControllerExclusion(t *testing.T) {
	for _, phase := range []string{"stopping", "complete"} {
		store, plan, targets, _ := providerFakeFixture(t, "radius", "external", "enforce")
		require.NoError(t, store.begin(t.Context(), plan, targets, true))
		cm, _, err := store.read(t.Context(), plan.Namespace)
		require.NoError(t, err)
		cm.Data[providerTransitionKey] = `{"version":1,"phase":"` + phase + `"}`
		_, err = store.kube.CoreV1().ConfigMaps(plan.Namespace).Update(t.Context(), cm, metav1.UpdateOptions{})
		require.NoError(t, err)
		client := store.kube.(*fake.Clientset)
		client.ClearActions()
		require.ErrorContains(t, store.begin(t.Context(), plan, targets, true), "legacy or unsupported")
		for _, action := range client.Actions() {
			require.Contains(t, []string{"get", "list"}, action.GetVerb())
		}
	}
	store, plan, targets, _ := providerFakeFixture(t, "radius", "external", "enforce")
	delete(targets, "controller")
	require.Error(t, store.begin(t.Context(), plan, targets, true), "controller must not silently disappear from the whole-fleet protocol")
}

func TestTLSProviderSelectedChartAndRotationReject(t *testing.T) {
	chart := certificateTestChart(t)
	delete(chart.Metadata.Annotations, "radius.io/provider-transition-protocol")
	options := ClusterOptions{TLS: TLSProviderOptions{Provider: "external"}}
	current := map[string]any{}
	require.NoError(t, unstructured.SetNestedField(current, true, "global", "rbac", "enabled"))
	_, _, err := (&Impl{}).prepareSelectedCertificates(t.Context(), options, chart, current, "", false)
	require.ErrorContains(t, err, "selected chart")
	options.Certificates.RotateCA = true
	_, _, err = (&Impl{}).prepareCertificateTarget(t.Context(), options, chart, current, "")
	require.ErrorContains(t, err, "cannot be combined")
}

func TestProviderTokenCSITargetValues(t *testing.T) {
	for _, from := range []string{"radius", "external"} {
		to := "radius"
		if from == to {
			to = "external"
		}
		source := &releasev1.Release{Chart: certificateTestChart(t), Config: providerTestValues(from)}
		source.Config["de"] = map[string]any{"image": "test.invalid/fixture-engine", "tag": "fixture",
			"internalAuth": map[string]any{"enabled": true}}
		replicas := map[string]int32{}
		for _, component := range rotationComponents {
			replicas[component] = 1
		}
		replicas["ucp"] = 2
		transaction := uuid.NewString()
		before := copyValues(source.Config)
		values, err := providerTargetValues(source, to, transaction, nil, replicas)
		require.NoError(t, err)
		require.Equal(t, before, source.Config)
		provider, _, err := providerValue(values)
		require.NoError(t, err)
		require.Equal(t, to, provider)
		generation, _, err := unstructured.NestedString(values, "global", "rbac", providerGenerationValue)
		require.NoError(t, err)
		require.Equal(t, strings.ReplaceAll(transaction, "-", ""), generation)
		count, _, err := unstructured.NestedInt64(values, "global", "rbac", "providerReplicas", "ucp")
		require.NoError(t, err)
		require.EqualValues(t, 2, count)
		require.NoError(t, unstructured.SetNestedField(source.Config, true, "de", "ucpMTLS", "enabled"))
		_, err = providerTargetValues(source, to, transaction, nil, replicas)
		require.ErrorContains(t, err, "legacy contract")
	}
}
