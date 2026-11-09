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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestProviderChartScopeHandoffPreservesCAHistory(t *testing.T) {
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, bootstrap := range []bool{false, true} {
			for _, target := range []string{"external", "radius"} {
				t.Run(fmt.Sprintf("%s/bootstrap=%t/target=%s", mode, bootstrap, target), func(t *testing.T) {
					source := "radius"
					if target == "radius" {
						source = "external"
					}
					_, ca, fixture := rotationIssuerFixture(t, source == "external", false)
					ca.Phase = rotationComplete
					if bootstrap {
						ca.SchemaVersion, ca.Bootstrap, ca.OldCA, ca.Plan.OldComponentCA = 3, true, nil, nil
					}
					settings := certificateValues(fixture.values)
					settings["caBundle"] = strings.TrimSpace(string(ca.NewCA))
					if mode != "issuerRef" {
						ca.Plan.ExternalIssuer = nil
						ca.Plan.TargetMode, ca.Plan.Issuer, ca.Plan.SigningSecret, ca.Plan.SigningUID = mode, "selected-ca", "operator-ca", "signer"
						settings = map[string]any{"mode": mode}
						if mode == "caSecret" {
							settings["caSecret"], settings["caBundle"] = ca.Plan.SigningSecret, strings.TrimSpace(string(ca.NewCA))
						}
					}
					setCertificateValues(fixture.values, settings)
					require.NoError(t, unstructured.SetNestedField(fixture.values, target, "global", "rbac", "mtls", "provider"))
					_, plan, _, _ := providerRecordingFixture(t, source, target)
					plan.Aggregation.JournalUID, plan.Aggregation.Revision = "ca-journal", ca.revision()
					plan.Aggregation.APIServiceUID, plan.Aggregation.Scope = ca.APIServiceUID, ca.Plan.effectiveScope()
					plan.Aggregation.TrustSHA256 = ca.Plan.TargetTrustSHA256
					var err error
					plan.Helm.TargetValuesSHA256, err = providerHash(fixture.values)
					require.NoError(t, err)
					state := providerTransitionState{Version: 1, Plan: plan, Phase: providerRecording}
					require.NoError(t, state.validate())
					caRaw, err := json.Marshal(ca)
					require.NoError(t, err)
					caJournal := &unstructured.Unstructured{Object: map[string]any{
						"apiVersion": "v1", "kind": "ConfigMap",
						"metadata": map[string]any{"name": rotationJournalName, "namespace": RadiusSystemNamespace,
							"uid": "ca-journal", "resourceVersion": "1", "labels": map[string]any{rotationOwner: radiusReleaseName}},
						"data": map[string]any{rotationJournalKey: string(caRaw)},
					}}
					providerJournal := &unstructured.Unstructured{Object: map[string]any{
						"apiVersion": "v1", "kind": "ConfigMap",
						"metadata": map[string]any{"name": providerTransitionJournal, "namespace": RadiusSystemNamespace,
							"uid": "provider-journal", "resourceVersion": "1", "labels": map[string]any{providerTransitionMarker: radiusReleaseName}},
					}}
					writeState := func() {
						t.Helper()
						raw, err := json.Marshal(state)
						require.NoError(t, err)
						require.NoError(t, unstructured.SetNestedStringMap(providerJournal.Object,
							map[string]string{providerTransitionKey: string(raw)}, "data"))
					}
					writeState()
					fixture.objects = append(fixture.objects, caJournal, providerJournal)
					for _, phase := range []providerTransitionPhase{providerRecording, providerComplete} {
						state.Phase = phase
						writeState()
						rendered, err := fixture.render(t)
						require.NoError(t, err)
						require.Equal(t, target == "radius", strings.Contains(rendered, "name: ucp-cert\n"))
						require.Equal(t, string(caRaw), caJournal.Object["data"].(map[string]any)[rotationJournalKey],
							"the historical CA scope, origin and predecessor must not be rewritten")
					}
					for _, phase := range []providerTransitionPhase{providerStopping, providerStaging, providerStarting, providerVerifying} {
						state.Phase = phase
						writeState()
						_, err := fixture.render(t)
						require.ErrorContains(t, err, "unfinished")
					}
					state.Phase = providerRecording
					writeState()
					require.NoError(t, unstructured.SetNestedField(fixture.values, "drift", "ucp", "image"))
					_, err = fixture.render(t)
					require.ErrorContains(t, err, "settings differ")
					unstructured.RemoveNestedField(fixture.values, "ucp")
					state.Plan.Aggregation.JournalUID = "replacement"
					writeState()
					_, err = fixture.render(t)
					require.ErrorContains(t, err, "same completed aggregation")
				})
			}
		}
	}
}
