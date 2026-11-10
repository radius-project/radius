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
	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestProviderTargetValuesPreserveCAAndUnrelatedSettings(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"radius", "external"} {
		for _, mode := range []string{"dryRun", "enforce"} {
			for _, certificateMode := range []string{"selfSigned", "caSecret", "issuerRef"} {
				t.Run(target+"/"+mode+"/"+certificateMode, func(t *testing.T) {
					sourceProvider, optIn := "radius", "externalIdentity"
					if target == "radius" {
						sourceProvider, optIn = "external", "ucpMTLS"
					}
					certificates := map[string]any{"mode": certificateMode, "caBundle": "public",
						"issuerRef": map[string]any{"name": "customer"}, "caSecret": "selected"}
					source := &releasev1.Release{Chart: &chart.Chart{Values: map[string]any{}},
						Config: map[string]any{"global": map[string]any{"rbac": map[string]any{
							"enabled": mode == "enforce", "dryRun": mode == "dryRun",
							"mtls": map[string]any{"provider": sourceProvider}, "certificates": certificates,
						}}, "de": map[string]any{"image": "test.invalid/paired", "tag": "verified"},
							"unrelated": map[string]any{"preserve": "yes"}}}
					before, err := providerHash(source)
					require.NoError(t, err)
					overrides := map[string]any{"de": map[string]any{optIn: map[string]any{"enabled": true}}}
					replicas := map[string]int32{"ucp": 3, "applications-rp": 2, "dynamic-rp": 4, "controller": 1, "bicep-de": 2}
					id := uuid.NewString()
					values, err := providerTargetValues(source, target, id, overrides, replicas)
					require.NoError(t, err)
					gotCA, _, err := unstructured.NestedMap(values, "global", "rbac", "certificates")
					require.NoError(t, err)
					require.Equal(t, certificates, gotCA)
					provider, _, err := providerValue(values)
					require.NoError(t, err)
					require.Equal(t, target, provider)
					require.Equal(t, source.Config["unrelated"], values["unrelated"])
					generation, _, err := unstructured.NestedString(values, "global", "rbac", providerGenerationValue)
					require.NoError(t, err)
					require.Equal(t, strings.ReplaceAll(id, "-", ""), generation)
					for _, component := range rotationComponents {
						count, _, err := unstructured.NestedInt64(values, "global", "rbac", "providerReplicas", component)
						require.NoError(t, err)
						require.Equal(t, int64(replicas[component]), count)
					}
					for _, path := range [][]string{{"global", "rbac", "startupProof", "enabled"}, {"de", "ucpMTLS", "startupProof", "enabled"}} {
						enabled, _, err := unstructured.NestedBool(values, path...)
						require.NoError(t, err)
						require.Equal(t, target == "radius", enabled)
					}
					after, err := providerHash(source)
					require.NoError(t, err)
					require.Equal(t, before, after)
					for _, bad := range []map[string]any{
						{"global": map[string]any{"rbac": map[string]any{"enabled": false}}},
						{"de": map[string]any{"image": "another"}},
						{"global": map[string]any{"rbac": map[string]any{"certificates": map[string]any{"mode": "selfSigned"}}}},
						{"de": map[string]any{optIn: map[string]any{"enabled": "true"}}},
						{"de": map[string]any{optIn: map[string]any{"enabled": false}}},
					} {
						_, err := providerTargetValues(source, target, id, bad, replicas)
						require.Error(t, err)
					}
					_, err = providerTargetValues(source, target, id, nil, replicas)
					require.ErrorContains(t, err, "explicit")
				})
			}
		}
	}
}

func TestProviderReplicaChartPreservesEntireFleet(t *testing.T) {
	t.Parallel()
	selected, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	options := PopulateDefaultClusterOptions(CLIClusterOptions{})
	counts := map[string]any{"ucp": int64(3), "applications-rp": int64(2), "dynamic-rp": int64(4), "controller": int64(1), "bicep-de": int64(2)}
	values := map[string]any{"global": map[string]any{"rbac": map[string]any{
		"providerGeneration": strings.Repeat("a", 32), "providerReplicas": counts,
	}}}
	impl := &Impl{contourCluster: &fakeContourCluster{}}
	prepared, err := impl.renderProviderPreparation(t.Context(), selected, options, values, "")
	require.NoError(t, err)
	for _, component := range rotationComponents {
		require.Equal(t, int32(counts[component].(int64)), *prepared.deployments[component].Spec.Replicas)
	}
	for _, bad := range []any{false, "1", map[string]any{}, int64(0), float64(1.5), int64(-1), int64(2147483648)} {
		broken := copyValues(values)
		require.NoError(t, unstructured.SetNestedField(broken, bad, "global", "rbac", "providerReplicas", "ucp"))
		_, err := impl.renderProviderPreparation(t.Context(), selected, options, broken, "")
		require.Error(t, err)
	}
	for _, bad := range []any{false, "invalid"} {
		broken := copyValues(values)
		require.NoError(t, unstructured.SetNestedField(broken, bad, "global", "rbac", "providerReplicas"))
		_, err := impl.renderProviderPreparation(t.Context(), selected, options, broken, "")
		require.Error(t, err)
	}
}
