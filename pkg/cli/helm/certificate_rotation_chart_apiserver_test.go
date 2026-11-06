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
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func installRotationTestCRDs(t *testing.T, store *rotationStore) {
	t.Helper()
	resource := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	for _, entry := range []struct{ kind, singular, plural string }{{"Certificate", "certificate", "certificates"}, {"Issuer", "issuer", "issuers"}} {
		name := entry.plural + ".cert-manager.io"
		_, err := store.dynamic.Resource(resource).Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
			"metadata": map[string]any{"name": name},
			"spec": map[string]any{"group": "cert-manager.io", "scope": "Namespaced",
				"names": map[string]any{"kind": entry.kind, "singular": entry.singular, "plural": entry.plural},
				"versions": []any{map[string]any{"name": "v1", "served": true, "storage": true,
					"schema": map[string]any{"openAPIV3Schema": map[string]any{"type": "object", "properties": map[string]any{
						"spec": map[string]any{"type": "object", "x-kubernetes-preserve-unknown-fields": true},
					}}}}},
			},
		}}, metav1.CreateOptions{})
		require.NoError(t, err)
		require.NoError(t, wait.PollUntilContextTimeout(t.Context(), 50*time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			object, err := store.dynamic.Resource(resource).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			raw, found, err := unstructured.NestedFieldNoCopy(object.Object, "status", "conditions")
			if err != nil {
				return false, err
			}
			if !found || raw == nil {
				return false, nil
			}
			conditions, _, err := unstructured.NestedSlice(object.Object, "status", "conditions")
			if err != nil {
				return false, err
			}
			for _, condition := range conditions {
				value, ok := condition.(map[string]any)
				if ok && value["type"] == "Established" && value["status"] == "True" {
					return true, nil
				}
			}
			return false, nil
		}))
	}
}

func TestRotationShippingWorkloadTrust(t *testing.T) {
	for _, ssa := range []bool{false, true} {
		t.Run(fmt.Sprintf("serverSideApply=%t", ssa), func(t *testing.T) {
			store, config, state := newPlannedRotationTest(t)
			installRotationTestCRDs(t, store)
			for _, phase := range rotationPhases[1:] {
				advanceRotationTest(t, store, phase)
				if phase == rotationAdopt {
					for _, workload := range state.Plan.Consumers {
						require.NoError(t, store.reconcileRotationDeployment(t.Context(), state.ID, workload))
					}
				}
			}
			api, err := store.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
			require.NoError(t, err)
			api.SetLabels(map[string]string{"app.kubernetes.io/managed-by": "Helm"})
			_, err = store.dynamic.Resource(apiServiceResource).Update(t.Context(), api, metav1.UpdateOptions{})
			require.NoError(t, err)
			kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
			require.NoError(t, clientcmd.WriteToFile(clientcmdapi.Config{
				Clusters:  map[string]*clientcmdapi.Cluster{"test": {Server: config.Host, CertificateAuthorityData: config.CAData}},
				AuthInfos: map[string]*clientcmdapi.AuthInfo{"test": {ClientCertificateData: config.CertData, ClientKeyData: config.KeyData}},
				Contexts:  map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test"}}, CurrentContext: "test",
			}, kubeconfig))
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig, flags.Namespace = &kubeconfig, &store.namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, store.namespace, helmDriverSecret))
			names := []string{"_helpers.tpl", "_rotation.tpl", "_tls-proof.tpl", "_aggregation-trust.tpl",
				"ucp/apiservice.yaml", "ucp/service.yaml", "ucp/deployment.yaml", "rp/deployment.yaml",
				"controller/deployment.yaml", "dynamic-rp/deployment.yaml", "dynamic-rp/buildkit-config.yaml",
				"de/deployment.yaml", "global/certificates.yaml"}
			chart, err := loader.Load("../../../deploy/Chart")
			require.NoError(t, err)
			selected := chart.Templates[:0]
			for _, template := range chart.Templates {
				if slices.Contains(names, strings.TrimPrefix(template.Name, "templates/")) {
					selected = append(selected, template)
				}
			}
			chart.Templates = selected
			values := map[string]any{
				"global": map[string]any{"prometheus": map[string]any{"enabled": true}, "rbac": map[string]any{
					"dryRun": true, "startupProof": map[string]any{"enabled": true, "expectedTrustSHA256": state.Plan.TargetTrustSHA256},
					"certificates": map[string]any{"mode": "caSecret", "caSecret": state.Plan.SigningSecret, "caBundle": string(state.NewCA)},
				}},
				"de": map[string]any{"ucpMTLS": map[string]any{"enabled": true, "startupProof": map[string]any{
					"enabled": true, "expectedTrustSHA256": state.Plan.TargetTrustSHA256,
				}}},
			}
			install := newInstallClient(configuration, store.release, store.namespace, false, time.Minute)
			// Establish Helm ownership before testing forced SSA upgrades.
			install.ServerSideApply = false
			_, err = install.Run(chart, values)
			if err != nil {
				require.ErrorContains(t, err, "the object has been modified")
			}
			for attempt := 0; attempt < 3; attempt++ {
				upgrade := newUpgradeClient(configuration, store.namespace, false, true, time.Minute, ssa)
				upgrade.ServerSideApply = fmt.Sprint(ssa)
				_, err = upgrade.Run(store.release, chart, map[string]any{})
				if err == nil {
					break
				}
				// The native aggregation status controller can update the target
				// while Helm applies the Service. Retry must render again.
				require.ErrorContains(t, err, "the object has been modified")
			}
			require.NoError(t, err)
			for _, workload := range state.Plan.Consumers {
				deployment, err := store.kube.AppsV1().Deployments(store.namespace).Get(t.Context(), workload.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, workload.UID, deployment.UID)
				require.NoError(t, rotationConsumerTemplate(deployment.Spec.Template, rotationConsumer{
					Name: workload.Name, TrustMap: state.Plan.TargetTrustMap, TrustSHA256: state.Plan.TargetTrustSHA256,
				}))
				require.Equal(t, state.ID+"/complete", deployment.Annotations[rotationRevision])
				if workload.Name == "controller" {
					require.Equal(t, appsv1.RecreateDeploymentStrategyType, deployment.Spec.Strategy.Type)
				}
				if workload.Name == "ucp" {
					require.Equal(t, state.Plan.ServingSecret, rotationServingSecret(deployment.Spec.Template.Spec))
				}
				certificate, err := store.dynamic.Resource(rotationCertificateResource).Namespace(store.namespace).Get(t.Context(), workload.Name+"-cert", metav1.GetOptions{})
				require.NoError(t, err)
				issuer, _, err := unstructured.NestedString(certificate.Object, "spec", "issuerRef", "name")
				require.NoError(t, err)
				require.Equal(t, state.Plan.Issuer, issuer)
				require.Equal(t, state.ID+"/complete", certificate.GetAnnotations()[rotationRevision])
			}
		})
	}
}
