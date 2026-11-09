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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

// Model an interrupted Helm install whose processes have converged. The existing
// controller fixture issues real leaves and loads a separate aggregation Source;
// this does not stand in for running cert-manager or published-image acceptance.
func TestBootstrapVerificationResume(t *testing.T) {
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, external := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/external=%t", mode, external), func(t *testing.T) {
				runner, original := aggregationExecutionFixture(t, external)
				s, ctx := runner.store, t.Context()
				if mode == "issuerRef" {
					original.NewCA = []byte(strings.TrimSpace(string(original.NewCA)))
				}
				original.Plan.ServingSecret = "ucp-cert-aggregation-" + strings.ReplaceAll(original.ID, "-", "")[:16]
				storeAggregationExecutionState(t, s, original)
				if !external {
					for _, name := range rotationComponents {
						deployment, err := s.kube.AppsV1().Deployments(s.namespace).Get(ctx, name, metav1.GetOptions{})
						require.NoError(t, err)
						portName := "metrics"
						if name == "bicep-de" {
							portName = "bicep-de-api"
						}
						deployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: portName, ContainerPort: 9090}}
						_, err = s.kube.AppsV1().Deployments(s.namespace).Update(ctx, deployment, metav1.UpdateOptions{})
						require.NoError(t, err)
					}
				}
				for _, phase := range []rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationRetire} {
					require.NoError(t, runner.run(ctx, original.ID, phase))
				}
				kube, dynamic := s.kube.(*kubefake.Clientset), s.dynamic.(*dynamicfake.FakeDynamicClient)
				require.NoError(t, kube.Tracker().Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: s.namespace}}))
				require.NoError(t, kube.CoreV1().ConfigMaps(s.namespace).Delete(ctx, rotationJournalName, metav1.DeleteOptions{}))
				api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
				require.NoError(t, err)
				annotations := api.GetAnnotations()
				delete(annotations, rotationJournalUID)
				delete(annotations, rotationRevision)
				delete(annotations, rotationOwner)
				annotations[aggregationBootstrapOwner] = original.ID
				api.SetAnnotations(annotations)
				require.NoError(t, dynamic.Tracker().Update(apiServiceResource, api, ""))
				selected, err := loader.Load("../../../deploy/Chart")
				require.NoError(t, err)
				values := copyValues(selected.Values)
				require.NoError(t, unstructured.SetNestedField(values, true, "global", "rbac", "dryRun"))
				require.NoError(t, unstructured.SetNestedField(values, true, "de", "ucpMTLS", "enabled"))
				require.NoError(t, unstructured.SetNestedField(values, true, "global", "prometheus", "enabled"))
				require.NoError(t, unstructured.SetNestedField(values, int64(9090), "global", "prometheus", "port"))
				if external {
					require.NoError(t, unstructured.SetNestedField(values, "external", "global", "rbac", "mtls", "provider"))
				}
				settings := map[string]any{"mode": mode}
				var issuer *rotationExternalIssuer
				kubeContext := ""
				if mode == "issuerRef" {
					_, pinned, policy := rotationIssuerFixture(t, external, false)
					issuer = pinned.Plan.ExternalIssuer
					require.NoError(t, dynamic.Tracker().Add(policy.object("Issuer", issuer.Name).DeepCopy()))
					settings = certificateValues(policy.values)
					settings["caBundle"] = string(original.NewCA)
					server := policy.server(t)
					kubeContext = useTargetKubeconfig(t, server.URL, server.URL, false)
				} else if mode == "caSecret" {
					settings["caSecret"], settings["caBundle"] = original.Plan.SigningSecret, string(original.NewCA)
				}
				setCertificateValues(values, settings)
				b := &aggregationBootstrapInstaller{impl: &Impl{}, store: s, chart: selected, values: values,
					options: NewDefaultClusterOptions(), kubeContext: kubeContext, runner: runner}
				requested, err := b.requested(ctx)
				require.NoError(t, err)
				state := requested
				state.ID, state.Phase, state.CA = original.ID, bootstrapVerifying, original.NewCA
				state.CAPinnedAt = s.now().UTC()
				state.TrustSHA256, state.ProofPort = original.Plan.TargetTrustSHA256, 9090
				if mode != "issuerRef" {
					state.Signer, state.SignerUID = original.Plan.SigningSecret, original.Plan.SigningUID
				}
				if mode == "selfSigned" {
					root, err := kube.CoreV1().Secrets(s.namespace).Get(ctx, state.Signer, metav1.GetOptions{})
					require.NoError(t, err)
					root.Name, root.UID = "radius-ca", "root-secret-uid"
					require.NoError(t, kube.Tracker().Add(root))
					state.RootSecretUID = root.UID
				}
				for _, name := range rotationComponents {
					if external && name != "ucp" {
						continue
					}
					deployment, err := kube.AppsV1().Deployments(s.namespace).Get(ctx, name, metav1.GetOptions{})
					require.NoError(t, err)
					deployment.Annotations[aggregationBootstrapOwner] = state.ID
					deployment.APIVersion, deployment.Kind = "apps/v1", "Deployment"
					data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(deployment)
					require.NoError(t, err)
					require.NoError(t, dynamic.Tracker().Add(&unstructured.Unstructured{Object: data}))
					if external {
						continue
					}
					certificate, err := dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(ctx, name+"-cert", metav1.GetOptions{})
					require.NoError(t, err)
					metadata := certificate.GetAnnotations()
					metadata[aggregationBootstrapOwner] = state.ID
					certificate.SetAnnotations(metadata)
					if issuer != nil {
						require.NoError(t, unstructured.SetNestedMap(certificate.Object,
							map[string]any{"group": issuer.Group, "kind": issuer.Kind, "name": issuer.Name}, "spec", "issuerRef"))
					}
					require.NoError(t, dynamic.Tracker().Update(rotationCertificateResource, certificate, s.namespace))
				}
				if issuer != nil {
					certificate, err := dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(ctx, original.Plan.ServingSecret, metav1.GetOptions{})
					require.NoError(t, err)
					require.NoError(t, unstructured.SetNestedMap(certificate.Object,
						map[string]any{"group": issuer.Group, "kind": issuer.Kind, "name": issuer.Name}, "spec", "issuerRef"))
					require.NoError(t, dynamic.Tracker().Update(rotationCertificateResource, certificate, s.namespace))
				}
				service, err := kube.CoreV1().Services(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
				require.NoError(t, err)
				service.Annotations[aggregationBootstrapOwner] = state.ID
				service.APIVersion, service.Kind = "v1", "Service"
				data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(service)
				require.NoError(t, err)
				require.NoError(t, dynamic.Tracker().Add(&unstructured.Unstructured{Object: data}))
				installedValues, err := b.installValues(state)
				require.NoError(t, err)
				state.InstallValuesSHA256, err = bootstrapValuesDigest(installedValues)
				require.NoError(t, err)
				require.NoError(t, state.validate(s.now()))
				raw, err := json.Marshal(state)
				require.NoError(t, err)
				_, err = kube.CoreV1().ConfigMaps(s.namespace).Create(ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: aggregationBootstrapName, Namespace: s.namespace,
						Labels: map[string]string{aggregationBootstrapOwner: s.release}},
					Data: map[string]string{rotationJournalKey: string(raw)},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
				failedCompletion := true
				kube.PrependReactor("update", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
					cm := action.(k8stesting.UpdateAction).GetObject().(*corev1.ConfigMap)
					if failedCompletion && cm.Name == aggregationBootstrapName && strings.Contains(cm.Data[rotationJournalKey], `"phase":"complete"`) {
						return true, nil, errors.New("lost final bootstrap write")
					}
					return false, nil, nil
				})
				if external {
					runner.observe = func(context.Context, *rest.Config, TLSProofTarget) (PodTLSProof, error) {
						t.Fatal("external bootstrap must not substitute component evidence for aggregation evidence")
						return PodTLSProof{}, nil
					}
				}
				probe := runner.probe
				probeContext, stopProbe := context.WithCancel(ctx)
				runner.probe = func(context.Context, *rest.Config, map[types.UID]bool) error {
					stopProbe()
					return errors.New("aggregated API has not accepted the selected CA")
				}
				require.ErrorContains(t, b.run(probeContext), "interrupted")
				runner.probe = probe
				_, pending, err := s.readBootstrap(ctx)
				require.NoError(t, err)
				require.Equal(t, bootstrapVerifying, pending.Phase)
				var driftSigner, restoreSigner func()
				if issuer != nil {
					resource := rotationIssuerResource
					namespace := s.namespace
					if issuer.Kind == "ClusterIssuer" {
						resource.Resource, namespace = "clusterissuers", ""
					}
					signer, err := dynamic.Resource(resource).Namespace(namespace).Get(ctx, issuer.Name, metav1.GetOptions{})
					require.NoError(t, err)
					driftSigner = func() {
						changed := signer.DeepCopy()
						changed.SetUID("replacement-issuer")
						require.NoError(t, dynamic.Tracker().Update(resource, changed, namespace))
					}
					restoreSigner = func() {
						require.NoError(t, dynamic.Tracker().Update(resource, signer, namespace))
					}
				} else {
					name := state.Signer
					if mode == "selfSigned" {
						name = "radius-ca"
					}
					signer, err := kube.CoreV1().Secrets(s.namespace).Get(ctx, name, metav1.GetOptions{})
					require.NoError(t, err)
					driftSigner = func() {
						changed := signer.DeepCopy()
						changed.UID = "replacement-signer"
						require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), changed, s.namespace))
					}
					restoreSigner = func() {
						require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), signer, s.namespace))
					}
				}
				drifted := false
				runner.probe = func(ctx context.Context, config *rest.Config, pods map[types.UID]bool) error {
					if err := probe(ctx, config, pods); err != nil {
						return err
					}
					driftSigner()
					drifted = true
					return nil
				}
				require.Error(t, b.run(ctx), "signer replacement during successful serving evidence must prevent completion")
				require.True(t, drifted)
				_, _, err = s.read(ctx)
				require.True(t, apierrors.IsNotFound(err), "signer drift must not create completed lifecycle evidence")
				restoreSigner()
				runner.probe = probe
				require.ErrorContains(t, b.run(ctx), "lost final bootstrap write")
				_, pending, err = s.readBootstrap(ctx)
				require.NoError(t, err)
				require.Equal(t, bootstrapVerifying, pending.Phase)
				_, err = s.aggregationTransitionSnapshot(ctx)
				require.ErrorContains(t, err, "bootstrap")
				failedCompletion = false
				require.NoError(t, b.run(ctx))
				_, complete, err := s.read(ctx)
				require.NoError(t, err)
				require.True(t, complete.Bootstrap)
				require.Empty(t, complete.OldCA)
				require.Empty(t, complete.Plan.OldComponentCA)
				snapshot, err := s.aggregationTransitionSnapshot(ctx)
				require.NoError(t, err)
				require.True(t, snapshot.Complete)
				b.values = valuesutil.CoalesceTables(copyValues(installedValues), copyValues(selected.Values))
				require.NoError(t, b.run(ctx), "exact persisted Helm settings can resume completion")
			})
		}
	}
}
