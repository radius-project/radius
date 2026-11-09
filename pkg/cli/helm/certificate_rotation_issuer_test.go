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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"helm.sh/helm/v4/pkg/chart/common"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/engine"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

func rotationIssuerFixture(t *testing.T, aggregationOnly, clusterIssuer bool) (*rotationRunner, rotationState, *issuerPolicyFixture) {
	t.Helper()
	runner, state := rotationRunnerFixture(t)
	workload, err := state.Plan.aggregationWorkload()
	require.NoError(t, err)
	state.SchemaVersion, state.Plan.Scope, state.Plan.Aggregation = 2, rotationComponentsAndAggregation, &workload
	if aggregationOnly {
		state = aggregationOnlyState(t, state)
	}
	fixture := newIssuerPolicyFixture(t)
	kind := "Issuer"
	if clusterIssuer {
		fixture.useClusterIssuer(t)
		kind = "ClusterIssuer"
	}
	issuer := fixture.object(kind, "external-ca")
	issuer.SetUID("external-issuer-uid")
	issuer.SetResourceVersion("1")
	specDigest, err := rotationIssuerSpecDigest(issuer)
	require.NoError(t, err)
	state.Plan.TargetMode, state.Plan.Issuer = "issuerRef", issuer.GetName()
	state.Plan.SigningSecret, state.Plan.SigningUID = "", ""
	state.Plan.ServingSecret = "ucp-cert-aggregation-" + strings.ReplaceAll(state.ID, "-", "")[:16]
	state.Plan.ExternalIssuer = &rotationExternalIssuer{
		Group: "cert-manager.io", Kind: kind, Name: issuer.GetName(), UID: issuer.GetUID(), SpecSHA256: specDigest,
		RequestPolicy: map[string]map[string]string{
			"certManager":    {"namespace": "cert-manager", "deployment": "cert-manager"},
			"approverPolicy": {"namespace": "cert-manager", "deployment": "cert-manager-approver-policy"},
			"webhook":        {"namespace": "cert-manager", "service": "cert-manager-webhook", "configuration": "cert-manager-webhook"},
		},
	}
	require.NoError(t, state.validate(time.Now()))
	require.NoError(t, runner.store.dynamic.(*dynamicfake.FakeDynamicClient).Tracker().Add(issuer.DeepCopy()))
	return runner, state, fixture
}

func TestCompletedRotationServingChartModes(t *testing.T) {
	for _, mode := range []string{"off", "dryRun", "enforce"} {
		t.Run(mode, func(t *testing.T) {
			_, state, fixture := rotationIssuerFixture(t, true, false)
			state.Phase = rotationComplete
			data, err := json.Marshal(state)
			require.NoError(t, err)
			fixture.objects = append(fixture.objects, &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "v1", "kind": "ConfigMap",
				"metadata": map[string]any{"name": rotationJournalName, "namespace": RadiusSystemNamespace,
					"uid": "journal-uid", "resourceVersion": "1", "labels": map[string]any{rotationOwner: radiusReleaseName}},
				"data": map[string]any{rotationJournalKey: string(data)},
			}})
			selected, err := loader.Load(filepath.Join("..", "..", "..", "deploy", "Chart"))
			require.NoError(t, err)
			files := []*common.File{}
			for _, file := range selected.Templates {
				if strings.HasSuffix(file.Name, ".tpl") || file.Name == "templates/ucp/configmaps.yaml" || file.Name == "templates/ucp/deployment.yaml" {
					files = append(files, file)
				}
			}
			selected.Templates = files
			values := valuesutil.CoalesceTables(copyValues(fixture.values), copyValues(selected.Values))
			require.NoError(t, unstructured.SetNestedField(values, mode == "enforce", "global", "rbac", "enabled"))
			require.NoError(t, unstructured.SetNestedField(values, mode == "dryRun", "global", "rbac", "dryRun"))
			require.NoError(t, unstructured.SetNestedField(values, true, "global", "prometheus", "enabled"))
			require.NoError(t, unstructured.SetNestedField(values, int64(state.Plan.Aggregation.ProofPort), "global", "prometheus", "port"))
			server := fixture.server(t)
			renderer := engine.New(&rest.Config{Host: server.URL})
			render := func() (map[string]string, error) {
				return renderer.Render(selected, common.Values{"Values": values, "Chart": selected.Metadata,
					"Release": map[string]any{"Name": radiusReleaseName, "Namespace": RadiusSystemNamespace}})
			}
			rendered, err := render()
			require.NoError(t, err)
			require.Contains(t, rendered["radius/templates/ucp/deployment.yaml"], "UCP_AGGREGATION_STARTUP_PROOF_ENABLED")
			require.Contains(t, rendered["radius/templates/ucp/deployment.yaml"], state.Plan.TargetTrustSHA256)
			var config corev1.ConfigMap
			require.NoError(t, yaml.Unmarshal([]byte(rendered["radius/templates/ucp/configmaps.yaml"]), &config))
			var settings map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(config.Data["ucp-config.yaml"]), &settings))
			require.Equal(t, mode, settings["authorization"].(map[string]any)["mode"])
			require.Equal(t, RadiusSystemNamespace, settings["authorization"].(map[string]any)["namespace"])
			require.NoError(t, unstructured.SetNestedField(values, false, "global", "prometheus", "enabled"))
			_, err = render()
			require.ErrorContains(t, err, "pinned metrics listener")
			require.NoError(t, unstructured.SetNestedField(values, true, "global", "prometheus", "enabled"))
			require.NoError(t, unstructured.SetNestedField(values, int64(state.Plan.Aggregation.ProofPort)+1, "global", "prometheus", "port"))
			_, err = render()
			require.ErrorContains(t, err, "pinned metrics listener")
		})
	}
}

func TestCompletedRotationAggregationChart(t *testing.T) {
	for _, scope := range []string{"legacy", "managed", "aggregation"} {
		for _, scenario := range []string{"complete", "unfinished", "APIService replaced", "trust changed"} {
			t.Run(scope+"/"+scenario, func(t *testing.T) {
				runner, state := rotationRunnerFixture(t)
				if scope != "legacy" {
					workload, err := state.Plan.aggregationWorkload()
					require.NoError(t, err)
					state.SchemaVersion, state.Plan.Scope, state.Plan.Aggregation = 2, rotationComponentsAndAggregation, &workload
					if scope == "aggregation" {
						state = aggregationOnlyState(t, state)
					}
				} else {
					state.Plan = nil
				}
				state.Phase = rotationComplete
				if scenario == "unfinished" {
					state.Phase = rotationIssue
				}
				fixture := newIssuerPolicyFixture(t)
				api, err := runner.store.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
				require.NoError(t, err)
				bundle := base64.StdEncoding.EncodeToString(state.NewCA)
				require.NoError(t, unstructured.SetNestedField(api.Object, bundle, "spec", "caBundle"))
				annotations := api.GetAnnotations()
				annotations[rotationOwner], annotations[rotationJournalUID], annotations[rotationRevision] = radiusReleaseName, "journal-uid", state.revision()
				api.SetAnnotations(annotations)
				switch scenario {
				case "APIService replaced":
					api.SetUID("replacement-uid")
				case "trust changed":
					require.NoError(t, unstructured.SetNestedField(api.Object, base64.StdEncoding.EncodeToString(state.OldCA), "spec", "caBundle"))
				}
				data, err := json.Marshal(state)
				require.NoError(t, err)
				fixture.objects = append(fixture.objects, api, &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "v1", "kind": "ConfigMap",
					"metadata": map[string]any{"name": rotationJournalName, "namespace": RadiusSystemNamespace,
						"uid": "journal-uid", "resourceVersion": "1", "labels": map[string]any{rotationOwner: radiusReleaseName}},
					"data": map[string]any{rotationJournalKey: string(data)},
				}})
				selected, err := loader.Load(filepath.Join("..", "..", "..", "deploy", "Chart"))
				require.NoError(t, err)
				helpers := []*common.File{}
				for _, file := range selected.Templates {
					if strings.HasSuffix(file.Name, ".tpl") {
						helpers = append(helpers, file)
					}
				}
				selected.Templates = append(helpers, &common.File{Name: "templates/aggregation-state.yaml",
					Data: []byte(`{{ include "radius.aggregationTrust" . }}`)})
				server := fixture.server(t)
				renderer := engine.New(&rest.Config{Host: server.URL})
				rendered, err := renderer.Render(selected, common.Values{"Values": fixture.values,
					"Release": map[string]any{"Name": radiusReleaseName, "Namespace": RadiusSystemNamespace}})
				if scenario != "complete" {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				var trust map[string]any
				require.NoError(t, json.Unmarshal([]byte(rendered["radius/templates/aggregation-state.yaml"]), &trust))
				require.Equal(t, bundle, trust["caBundle"])
			})
		}
	}
}

func TestCompletedIssuerRotationChart(t *testing.T) {
	for _, aggregationOnly := range []bool{false, true} {
		for _, clusterIssuer := range []bool{false, true} {
			for _, scenario := range []string{"complete", "unfinished", "unknown version", "legacy with scope", "wrong scope", "missing serving identity", "private signing reference", "changed public trust", "replaced issuer", "changed issuer specification"} {
				t.Run(fmt.Sprintf("aggregationOnly=%t/clusterIssuer=%t/%s", aggregationOnly, clusterIssuer, scenario), func(t *testing.T) {
					_, state, fixture := rotationIssuerFixture(t, aggregationOnly, clusterIssuer)
					state.Phase = rotationComplete
					certificates := certificateValues(fixture.values)
					certificates["caBundle"] = strings.TrimSpace(string(state.NewCA))
					setCertificateValues(fixture.values, certificates)
					if aggregationOnly {
						require.NoError(t, unstructured.SetNestedField(fixture.values, "external", "global", "rbac", "mtls", "provider"))
						fixture.forbiddenPath = "/apis/cert-manager.io/v1/namespaces/radius-system/certificates/applications-rp-cert"
					}
					switch scenario {
					case "unfinished":
						state.Phase = rotationIssue
					case "unknown version":
						state.SchemaVersion = 3
					case "legacy with scope":
						state.SchemaVersion = 1
					case "wrong scope":
						state.Plan.Scope = "unknown"
					case "missing serving identity":
						state.Plan.Aggregation = nil
					case "private signing reference":
						state.Plan.SigningSecret = "operator-key"
					case "changed public trust":
						next, _, _ := testSigningCA(t, nil, nil, nil, nil)
						certificates["caBundle"] = strings.TrimSpace(string(next))
						setCertificateValues(fixture.values, certificates)
					case "replaced issuer":
						fixture.object(state.Plan.ExternalIssuer.Kind, state.Plan.Issuer).SetUID("replacement-uid")
					case "changed issuer specification":
						fixture.set(t, state.Plan.ExternalIssuer.Kind, state.Plan.Issuer, "another-key", "spec", "ca", "secretName")
					}
					data, err := json.Marshal(state)
					require.NoError(t, err)
					fixture.objects = append(fixture.objects, &unstructured.Unstructured{Object: map[string]any{
						"apiVersion": "v1", "kind": "ConfigMap",
						"metadata": map[string]any{"name": rotationJournalName, "namespace": RadiusSystemNamespace,
							"uid": "journal-uid", "resourceVersion": "1", "labels": map[string]any{rotationOwner: radiusReleaseName}},
						"data": map[string]any{rotationJournalKey: string(data)},
					}})
					rendered, err := fixture.render(t)
					if scenario != "complete" {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
					if aggregationOnly {
						require.NotContains(t, rendered, "kind: Certificate")
					} else {
						require.Equal(t, 5, strings.Count(rendered, "kind: Certificate"))
						require.Equal(t, 5, strings.Count(rendered, "kind: "+state.Plan.ExternalIssuer.Kind+"\n"))
					}
				})
			}
		}
	}
}

func TestRotationIssuerJournalValidation(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		edit func(*rotationState)
	}{
		{"legacy schema", func(s *rotationState) { s.SchemaVersion = 1 }},
		{"signing Secret", func(s *rotationState) { s.Plan.SigningSecret = "private-ca" }},
		{"signing UID", func(s *rotationState) { s.Plan.SigningUID = "private-uid" }},
		{"missing public identity", func(s *rotationState) { s.Plan.ExternalIssuer = nil }},
		{"missing issuer UID", func(s *rotationState) { s.Plan.ExternalIssuer.UID = "" }},
		{"missing configuration pin", func(s *rotationState) { s.Plan.ExternalIssuer.SpecSHA256 = "" }},
		{"foreign group", func(s *rotationState) { s.Plan.ExternalIssuer.Group = "foreign.example" }},
		{"foreign kind", func(s *rotationState) { s.Plan.ExternalIssuer.Kind = "ExternalIssuer" }},
		{"mismatched issuer", func(s *rotationState) { s.Plan.Issuer = "other" }},
		{"legacy request name", func(s *rotationState) {
			s.Plan.ServingSecret = "radius-aggregation-" + strings.ReplaceAll(s.ID, "-", "")[:16]
		}},
		{"unknown policy role", func(s *rotationState) { s.Plan.ExternalIssuer.RequestPolicy["other"] = map[string]string{} }},
		{"extra policy field", func(s *rotationState) { s.Plan.ExternalIssuer.RequestPolicy["webhook"]["extra"] = "value" }},
		{"missing policy field", func(s *rotationState) { delete(s.Plan.ExternalIssuer.RequestPolicy["webhook"], "configuration") }},
		{"invalid prerequisite namespace", func(s *rotationState) { s.Plan.ExternalIssuer.RequestPolicy["webhook"]["namespace"] = "a.b" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, state, _ := rotationIssuerFixture(t, true, false)
			encoded, err := json.Marshal(state)
			require.NoError(t, err)
			decoded, err := decodeRotationState(string(encoded))
			require.NoError(t, err)
			require.NoError(t, decoded.validate(time.Now()))
			require.NotContains(t, string(encoded), "signingSecret")
			require.NotContains(t, string(encoded), "signingUID")
			tt.edit(&decoded)
			require.Error(t, decoded.validate(time.Now()))
		})
	}
}

func TestRotationIssuerNeverFallsBackToPrivateCA(t *testing.T) {
	t.Parallel()
	runner, state, fixture := rotationIssuerFixture(t, true, false)
	certificateValues(fixture.values)["caBundle"] = string(state.NewCA)
	impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) {
		t.Fatal("external issuer selection must not read a signing Secret")
		return nil, errors.New("unexpected private CA access")
	}}
	kube := runner.store.kube.(*kubefake.Clientset)
	kube.ClearActions()
	bundle, issuer, err := impl.rotationIssuerTarget(t.Context(), runner.store, NewDefaultClusterOptions(), fixture.values, "context")
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(string(state.NewCA)), string(bundle))
	require.Equal(t, state.Plan.ExternalIssuer, issuer)
	require.Empty(t, kube.Actions())
	ca, err := currentRotationComponentCA(t.Context(), &rotationStore{}, fixture.values, rotationState{}, false)
	require.NoError(t, err)
	require.Equal(t, state.NewCA, ca)
}

func TestRotationIssuerTargetPreservesPolicyReferences(t *testing.T) {
	t.Parallel()
	for _, cluster := range []bool{false, true} {
		t.Run(fmt.Sprint(cluster), func(t *testing.T) {
			t.Parallel()
			runner, state, fixture := rotationIssuerFixture(t, true, cluster)
			current := copyValues(fixture.values)
			certificates := certificateValues(current)
			certificates["mode"], certificates["caSecret"] = "caSecret", "old-signing-secret"
			delete(certificates, "issuerRef")
			setCertificateValues(current, certificates)
			before := copyValues(current)
			options := NewDefaultClusterOptions()
			options.Certificates.CertIssuer = state.Plan.ExternalIssuer.Kind + "/" + state.Plan.Issuer
			options.Certificates.CABundle = filepath.Join(t.TempDir(), "public.pem")
			require.NoError(t, os.WriteFile(options.Certificates.CABundle, state.NewCA, 0600))
			kube := runner.store.kube.(*kubefake.Clientset)
			kube.ClearActions()
			bundle, issuer, err := (&Impl{}).rotationIssuerTarget(t.Context(), runner.store, options, current, "context")
			require.NoError(t, err)
			require.Equal(t, state.Plan.ExternalIssuer, issuer)
			require.Equal(t, strings.TrimSpace(string(state.NewCA)), string(bundle))
			require.Equal(t, before, current)
			require.Empty(t, kube.Actions())
			certificates["requestPolicy"] = map[string]any{}
			setCertificateValues(current, certificates)
			_, _, err = (&Impl{}).rotationIssuerTarget(t.Context(), runner.store, options, current, "context")
			require.ErrorContains(t, err, "public policy references")
		})
	}
}

func TestRotationPublicIssuerAggregationRecoveryPlanning(t *testing.T) {
	t.Parallel()
	runner, original, _ := rotationIssuerFixture(t, true, true)
	s, ctx := runner.store, t.Context()
	storeAggregationExecutionState(t, s, original)
	kube := s.kube.(*kubefake.Clientset)
	deployment, err := kube.AppsV1().Deployments(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	deployment.Spec.ProgressDeadlineSeconds = ptr.To(int32(600))
	deployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}
	require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), deployment, s.namespace))
	client := s.dynamic.(*dynamicfake.FakeDynamicClient)
	kube.ClearActions()
	client.ClearActions()
	selection, err := planRotationResources(ctx, s, nil, uuid.NewString(), "", "issuerRef",
		nil, original.NewCA, nil, &original, original.Plan.ExternalIssuer, rotationAggregationOnly)
	require.NoError(t, err)
	require.Equal(t, 2, selection.SchemaVersion)
	require.Empty(t, selection.Plan.SigningSecret)
	require.Empty(t, selection.Plan.SigningUID)
	require.Empty(t, selection.Plan.Consumers)
	require.Empty(t, selection.Plan.OldComponentCA)
	require.Equal(t, original.Plan.ExternalIssuer, selection.Plan.ExternalIssuer)
	require.NoError(t, selection.validate(time.Now()))
	for _, action := range append(kube.Actions(), client.Actions()...) {
		require.Equal(t, "get", action.GetVerb())
		require.NotContains(t, []string{"secrets", "certificates"}, action.GetResource().Resource)
		if action.GetResource().Resource == "deployments" {
			require.Equal(t, "ucp", action.(k8stesting.GetAction).GetName())
		}
	}
}

func TestRotationIssuerReferenceIdentity(t *testing.T) {
	t.Parallel()
	local := map[string]any{"group": "cert-manager.io", "kind": "Issuer", "name": "same"}
	require.True(t, rotationIssuerRefsEqual(local, map[string]any{"name": "same"}))
	require.False(t, rotationIssuerRefsEqual(local, map[string]any{"name": "same", "kind": "ClusterIssuer"}))
	require.False(t, rotationIssuerRefsEqual(local, map[string]any{"name": "same", "group": "foreign.example"}))
}

func TestRotationIssuerReadOnlyFences(t *testing.T) {
	t.Parallel()
	for _, cluster := range []bool{false, true} {
		t.Run(map[bool]string{false: "Issuer", true: "ClusterIssuer"}[cluster], func(t *testing.T) {
			t.Parallel()
			runner, state, fixture := rotationIssuerFixture(t, true, cluster)
			store, ctx := runner.store, t.Context()
			kube := store.kube.(*kubefake.Clientset)
			client := store.dynamic.(*dynamicfake.FakeDynamicClient)
			kube.ClearActions()
			require.ErrorContains(t, store.checkPlannedSigner(ctx, state), "live chart policy")
			store.verifyIssuerPolicy = func(context.Context, rotationState) error { return errors.New("policy changed") }
			require.ErrorContains(t, store.checkPlannedSigner(ctx, state), "policy changed")
			store.verifyIssuerPolicy = func(context.Context, rotationState) error { return nil }
			require.NoError(t, store.checkPlannedSigner(ctx, state))
			issuer := fixture.object(state.Plan.ExternalIssuer.Kind, state.Plan.Issuer).DeepCopy()
			resource := rotationIssuerResource
			if cluster {
				resource.Resource = "clusterissuers"
			}
			store.verifyIssuerPolicy = func(context.Context, rotationState) error {
				issuer.Object["spec"] = map[string]any{"vault": map[string]any{"server": "https://changed.example"}}
				return client.Tracker().Update(resource, issuer, issuer.GetNamespace())
			}
			require.ErrorContains(t, store.checkPlannedSigner(ctx, state), "specification changed")
			require.Empty(t, kube.Actions(), "external signing validation must never read a private Secret")
		})
	}
}

func TestRotationIssuerCertificateWritesRequirePolicy(t *testing.T) {
	t.Parallel()
	runner, state, fixture := rotationIssuerFixture(t, false, true)
	store, ctx := runner.store, t.Context()
	journal, _, err := store.read(ctx)
	require.NoError(t, err)
	state.Phase = rotationIssue
	data, err := json.Marshal(state)
	require.NoError(t, err)
	journal.Data[rotationJournalKey] = string(data)
	_, err = store.kube.CoreV1().ConfigMaps(store.namespace).Update(ctx, journal, metav1.UpdateOptions{})
	require.NoError(t, err)
	client := store.dynamic.(*dynamicfake.FakeDynamicClient)
	client.ClearActions()
	require.ErrorContains(t, store.ensureRotationIssuer(ctx, state.ID), "live chart policy")
	require.ErrorContains(t, store.ensureAggregationCertificate(ctx, state.ID), "live chart policy")
	require.ErrorContains(t, store.switchRotationCertificate(ctx, state.ID, state.Plan.Consumers[0]), "live chart policy")
	for _, action := range client.Actions() {
		require.Equal(t, "get", action.GetVerb())
	}
	certificate, err := client.Resource(rotationCertificateResource).Namespace(store.namespace).Get(ctx, state.Plan.Consumers[0].Name+"-cert", metav1.GetOptions{})
	require.NoError(t, err)
	client = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), certificate, fixture.object("ClusterIssuer", "external-ca").DeepCopy())
	store.dynamic = client
	store.verifyIssuerPolicy = func(context.Context, rotationState) error { return nil }
	require.NoError(t, store.ensureRotationIssuer(ctx, state.ID))
	require.NoError(t, store.ensureAggregationCertificate(ctx, state.ID))
	require.NoError(t, store.switchRotationCertificate(ctx, state.ID, state.Plan.Consumers[0]))
	for _, name := range []string{state.Plan.ServingSecret, state.Plan.Consumers[0].Name + "-cert"} {
		certificate, err := client.Resource(rotationCertificateResource).Namespace(store.namespace).Get(ctx, name, metav1.GetOptions{})
		require.NoError(t, err)
		ref, _, err := unstructured.NestedMap(certificate.Object, "spec", "issuerRef")
		require.NoError(t, err)
		require.Equal(t, state.Plan.issuerReference(), ref)
	}
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "issuers" || action.GetResource().Resource == "clusterissuers" {
			require.Equal(t, "get", action.GetVerb(), "operator issuer must never be changed")
		}
	}
}

func TestRotationIssuerLivePolicy(t *testing.T) {
	for _, tt := range []struct {
		name                                                       string
		aggregationOnly, clusterIssuer, pending, drift, failPolicy bool
	}{
		{name: "managed"},
		{name: "aggregation only", aggregationOnly: true},
		{name: "cluster issuer", clusterIssuer: true},
		{name: "aggregation cluster issuer", aggregationOnly: true, clusterIssuer: true},
		{name: "pending history", pending: true},
		{name: "changed baseline", drift: true},
		{name: "live policy failure", failPolicy: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, state, fixture := rotationIssuerFixture(t, tt.aggregationOnly, tt.clusterIssuer)
			if tt.aggregationOnly {
				objects := fixture.objects[:0]
				for _, object := range fixture.objects {
					if object.GetKind() != "CertificateRequestPolicy" || object.GetName() == "radius-radius-system-ucp" {
						objects = append(objects, object)
					}
				}
				fixture.objects = objects
			}
			if tt.failPolicy {
				fixture.container("Deployment", "cert-manager")["args"] = []any{"--controllers=*"}
			}
			target := fixture.server(t)
			decoy := k8sutil.NewForbiddenAPIServer(t)
			kubeContext := useTargetKubeconfig(t, target.URL, decoy.URL, true)
			selected, err := loader.Load(filepath.Join("..", "..", "..", "deploy", "Chart"))
			require.NoError(t, err)
			selected.Templates = append(selected.Templates, &common.File{Name: "templates/no-workload-render.yaml",
				Data: []byte(`{{ fail "rotation must not render ordinary workload mutations" }}`)})
			originalTemplates, err := json.Marshal(selected.Templates)
			require.NoError(t, err)
			options := NewDefaultClusterOptions()
			options.RadiusInstalled = true
			options.RadiusReleaseValues = copyValues(fixture.values)
			setCertificateValues(options.RadiusReleaseValues, map[string]any{"mode": "caSecret", "caSecret": "old-operator-ca", "caBundle": string(state.Plan.OldComponentCA)})
			baseline := copyValues(options.RadiusReleaseValues)
			if tt.drift {
				baseline["changed"] = true
			}
			history := []*releasev1.Release{{Name: radiusReleaseName, Namespace: RadiusSystemNamespace, Version: 1,
				Chart: selected, Config: baseline, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}}}
			if tt.pending {
				history = append(history, &releasev1.Release{Name: radiusReleaseName, Namespace: RadiusSystemNamespace, Version: 2,
					Chart: selected, Config: baseline, Info: &releasev1.Info{Status: releasecommon.StatusPendingUpgrade}})
			}
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(history, nil)
			impl := &Impl{Helm: client}
			current := copyValues(fixture.values)
			before := copyValues(current)
			err = impl.checkRotationIssuerPolicy(t.Context(), options, selected, nil, current, kubeContext, state)
			switch {
			case tt.drift:
				require.ErrorContains(t, err, "changed since preflight")
			case tt.pending, tt.failPolicy:
				require.ErrorContains(t, err, "live issuer-policy verification failed")
			default:
				require.NoError(t, err)
			}
			require.Equal(t, before, current)
			retainedTemplates, err := json.Marshal(selected.Templates)
			require.NoError(t, err)
			require.Equal(t, originalTemplates, retainedTemplates)
			require.Empty(t, decoy.Requests())
		})
	}
}
