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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/engine"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
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

func TestLifecycleShippingSnapshot(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for isolated lifecycle projection tests.")
	}
	kubeconfig, kube := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	store := &rotationStore{kube: kube, dynamic: client, namespace: RadiusSystemNamespace, release: radiusReleaseName, now: time.Now}
	installRotationTestCRDs(t, store)
	state, journal := lifecycleFixture(t)
	state.Phase = rotationComplete
	_, err = kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: state.Namespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	journal.UID, journal.ResourceVersion = "", ""
	journal, err = kube.CoreV1().ConfigMaps(state.Namespace).Create(t.Context(), journal, metav1.CreateOptions{})
	require.NoError(t, err)
	for name, plane := range state.Planes {
		plane.Source.Bundle = plane.NewCA
		plane.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(plane.NewCA))
		plane.Issuer.Name = RadiusCAIssuerName
		if name == "aggregation" {
			plane.Issuer.Name = RadiusAPIServerIssuerName
		}
		_, err = client.Resource(rotationIssuerResource).Namespace(state.Namespace).Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
			"metadata": map[string]any{"name": plane.Issuer.Name, "namespace": state.Namespace},
			"spec":     map[string]any{"ca": map[string]any{"secretName": plane.Signer.Name}},
		}}, metav1.CreateOptions{})
		require.NoError(t, err)
		plane.TargetIssuer, err = store.snapshotCertificateIssuer(t.Context(), plane.Issuer)
		require.NoError(t, err)
		metadata := metav1.ObjectMeta{Name: plane.Source.Name, Namespace: state.Namespace,
			Labels: map[string]string{helmManagedByLabel: helmManagedByValue},
			Annotations: map[string]string{helmReleaseNameAnnotation: state.Release, helmReleaseNamespaceAnnotation: state.Namespace,
				rotationJournalUID: string(journal.UID), rotationRevision: state.ID + "/complete"}}
		if name == "internal" {
			source, createErr := kube.CoreV1().ConfigMaps(state.Namespace).Create(t.Context(), &corev1.ConfigMap{
				ObjectMeta: metadata, Data: map[string]string{caCertificateKey: string(plane.NewCA)},
			}, metav1.CreateOptions{})
			require.NoError(t, createErr)
			plane.Source.UID, plane.Source.ResourceVersion = source.UID, source.ResourceVersion
		} else {
			metadata.Annotations[allowDirectInjectionAnnotation] = "true"
			source, createErr := kube.CoreV1().Secrets(state.Namespace).Create(t.Context(), &corev1.Secret{
				ObjectMeta: metadata, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{caCertificateKey: plane.NewCA},
			}, metav1.CreateOptions{})
			require.NoError(t, createErr)
			plane.Source.UID, plane.Source.ResourceVersion = source.UID, source.ResourceVersion
		}
		plane.Previous.Source.UID = plane.Source.UID
		state.Planes[name] = plane
	}
	require.NoError(t, state.validate(time.Now()))
	writeJournal := func(selected certificateLifecycle) {
		data, marshalErr := json.Marshal(selected)
		require.NoError(t, marshalErr)
		journal.Data = map[string]string{rotationJournalKey: string(data)}
		journal, err = kube.CoreV1().ConfigMaps(state.Namespace).Update(t.Context(), journal, metav1.UpdateOptions{})
		require.NoError(t, err)
	}
	writeJournal(state)
	selected, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	templates := selected.Templates[:0]
	for _, file := range selected.Templates {
		if strings.HasSuffix(file.Name, ".tpl") {
			templates = append(templates, file)
		}
	}
	selected.Templates = append(templates, &common.File{Name: "templates/snapshot.json",
		Data: []byte(`{{ include "radius.lifecycleSnapshot" . }}`)})
	renderer := engine.New(config)
	render := func() (map[string]map[string]any, error) {
		output, renderErr := renderer.Render(selected, common.Values{"Values": selected.Values, "Chart": selected.Metadata,
			"Release": map[string]any{"Name": state.Release, "Namespace": state.Namespace}})
		if renderErr != nil {
			return nil, renderErr
		}
		var snapshot map[string]map[string]any
		decodeErr := json.Unmarshal([]byte(output[selected.Name()+"/templates/snapshot.json"]), &snapshot)
		return snapshot, decodeErr
	}
	for _, metadataUpdate := range []bool{false, true} {
		t.Run(fmt.Sprintf("metadataUpdate=%t", metadataUpdate), func(t *testing.T) {
			source, readErr := kube.CoreV1().Secrets(state.Namespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
			require.NoError(t, readErr)
			if metadataUpdate {
				source.Annotations["unrelated"] = "metadata"
				source, err = kube.CoreV1().Secrets(state.Namespace).Update(t.Context(), source, metav1.UpdateOptions{})
				require.NoError(t, err)
				require.NotEqual(t, state.Planes["aggregation"].Source.ResourceVersion, source.ResourceVersion)
				issuer, readErr := client.Resource(rotationIssuerResource).Namespace(state.Namespace).Get(t.Context(), RadiusAPIServerIssuerName, metav1.GetOptions{})
				require.NoError(t, readErr)
				issuer.SetAnnotations(map[string]string{"unrelated": "metadata"})
				_, err = client.Resource(rotationIssuerResource).Namespace(state.Namespace).Update(t.Context(), issuer, metav1.UpdateOptions{})
				require.NoError(t, err)
			}
			snapshot, renderErr := render()
			require.NoError(t, renderErr)
			for name, plane := range state.Planes {
				require.Equal(t, string(plane.NewCA), snapshot[name]["bundle"])
				require.Equal(t, string(plane.Source.UID), snapshot[name]["uid"])
			}
			require.Equal(t, source.ResourceVersion, snapshot["aggregation"]["resourceVersion"])
		})
	}
	for _, scenario := range []string{"legacy", "active", "replaced", "changed content", "staged issuer", "issuer replaced", "issuer configuration", "retained overlap"} {
		t.Run(scenario, func(t *testing.T) {
			bad := state
			bad.Planes = make(map[string]certificateLifecyclePlane, len(state.Planes))
			for name, plane := range state.Planes {
				bad.Planes[name] = plane
			}
			plane := bad.Planes["aggregation"]
			switch scenario {
			case "legacy":
				bad.SchemaVersion = 3
			case "active":
				bad.Phase = rotationPublish
			case "replaced":
				plane.Source.UID = "replaced"
			case "changed content":
				plane.Source.SHA256 = strings.Repeat("0", 64)
			case "staged issuer":
				plane.Issuer.Name = "distinct-staged-issuer"
				plane.TargetIssuer.Reference = plane.Issuer
			case "issuer replaced":
				plane.TargetIssuer.UID = "replaced"
			case "issuer configuration":
				plane.TargetIssuer.SpecSHA256 = strings.Repeat("0", 64)
			case "retained overlap":
				plane.NewCA = append(append([]byte{}, plane.OldCA...), plane.NewCA...)
			}
			bad.Planes["aggregation"] = plane
			writeJournal(bad)
			_, renderErr := render()
			require.Error(t, renderErr)
		})
	}
}
