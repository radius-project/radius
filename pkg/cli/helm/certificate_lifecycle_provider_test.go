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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// completedLifecycleFixture supplies completed journal evidence, not execution
// or controller acceptance. Adapter checks must remain public identity checks.
func completedLifecycleFixture(t *testing.T) *certificateExecutionWriter {
	t.Helper()
	w := lifecycleExecutionFixture(t)
	require.NoError(t, withIdentityOperation(t.Context(), w.store.kube, w.store.namespace, func(operation *identityOperation) error {
		w.operation = operation
		require.NoError(t, w.save(t.Context(), w.state))
		require.NoError(t, w.ensureResource(t.Context(), 0))
		return nil
	}))
	for name, plane := range w.state.Planes {
		var metadata metav1.Object
		if name == "internal" {
			source, err := w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Get(t.Context(), plane.Source.Name, metav1.GetOptions{})
			require.NoError(t, err)
			source.Data[caCertificateKey] = string(plane.NewCA)
			source.Annotations[rotationJournalUID], source.Annotations[rotationRevision] = string(w.journal.UID), w.state.ID+"/complete"
			metadata, err = w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Update(t.Context(), source, metav1.UpdateOptions{})
			require.NoError(t, err)
		} else {
			source, err := w.store.kube.CoreV1().Secrets(w.store.namespace).Get(t.Context(), plane.Source.Name, metav1.GetOptions{})
			require.NoError(t, err)
			source.Data[caCertificateKey] = plane.NewCA
			source.Annotations[rotationJournalUID], source.Annotations[rotationRevision] = string(w.journal.UID), w.state.ID+"/complete"
			metadata, err = w.store.kube.CoreV1().Secrets(w.store.namespace).Update(t.Context(), source, metav1.UpdateOptions{})
			require.NoError(t, err)
		}
		plane.Source.Bundle, plane.Source.ResourceVersion = plane.NewCA, metadata.GetResourceVersion()
		plane.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(plane.NewCA))
		plane.Source.JournalUID, plane.Source.Revision = w.journal.UID, w.state.ID+"/complete"
		w.state.Planes[name] = plane
	}
	w.state.Phase = rotationComplete
	w.state.Execution.FinalReleaseVersion = w.state.Execution.BaselineVersion + 1
	require.NoError(t, w.state.validate(w.store.now()))
	data, err := json.Marshal(w.state)
	require.NoError(t, err)
	w.journal.Data[rotationJournalKey] = string(data)
	w.journal, err = w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Update(t.Context(), w.journal, metav1.UpdateOptions{})
	require.NoError(t, err)
	api, err := w.store.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	api.Object["spec"].(map[string]any)["caBundle"] = base64.StdEncoding.EncodeToString(w.state.Planes["aggregation"].NewCA)
	_, err = w.store.dynamic.Resource(apiServiceResource).Update(t.Context(), api, metav1.UpdateOptions{})
	require.NoError(t, err)
	return w
}

func TestLifecycleGuardRejectsOrphanedPublicSources(t *testing.T) {
	for _, plane := range []string{"internal", "aggregation"} {
		t.Run(plane, func(t *testing.T) {
			w := completedLifecycleFixture(t)
			require.NoError(t, w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Delete(t.Context(), rotationJournalName, metav1.DeleteOptions{}))
			if plane == "aggregation" {
				require.NoError(t, w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Delete(t.Context(), "radius-mtls-ca", metav1.DeleteOptions{}))
			}
			impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return w.store, nil }}
			require.ErrorContains(t, impl.guardCertificateRotation(t.Context(), "", w.store.namespace, "upgrade", nil), "missing its public journal")
		})
	}
}

func TestLifecycleProviderAdapter(t *testing.T) {
	for _, scenario := range []string{"unchanged", "issuer status", "source metadata", "issuer replaced", "source replaced", "extra anchor", "insecure aggregation", "pending bootstrap", "incomplete"} {
		t.Run(scenario, func(t *testing.T) {
			w := completedLifecycleFixture(t)
			snapshot, err := w.store.aggregationTransitionSnapshot(t.Context())
			require.NoError(t, err)
			require.True(t, snapshot.Complete)
			require.Equal(t, w.journal.UID, snapshot.JournalUID)
			require.Equal(t, w.state.ID+"/complete", snapshot.Revision)
			switch scenario {
			case "issuer status", "issuer replaced":
				issuer, err := w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace).Get(t.Context(),
					w.state.Planes["aggregation"].Issuer.Name, metav1.GetOptions{})
				require.NoError(t, err)
				if scenario == "issuer replaced" {
					issuer.SetUID("replacement")
				} else {
					issuer.Object["status"] = map[string]any{"ready": true}
				}

				_, err = w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace).Update(t.Context(), issuer, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "source metadata", "source replaced":
				source, err := w.store.kube.CoreV1().Secrets(w.store.namespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
				require.NoError(t, err)
				if scenario == "source replaced" {
					source.UID = "replacement"
				} else {
					source.Annotations["unrelated"] = "metadata"
				}
				_, err = w.store.kube.CoreV1().Secrets(w.store.namespace).Update(t.Context(), source, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "extra anchor", "insecure aggregation":
				api, err := w.store.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
				require.NoError(t, err)
				if scenario == "extra anchor" {
					bundle := append(append([]byte{}, w.state.Planes["aggregation"].NewCA...), w.state.Planes["aggregation"].OldCA...)
					api.Object["spec"].(map[string]any)["caBundle"] = base64.StdEncoding.EncodeToString(bundle)
				} else {
					api.Object["spec"].(map[string]any)["insecureSkipTLSVerify"] = true
				}
				_, err = w.store.dynamic.Resource(apiServiceResource).Update(t.Context(), api, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "pending bootstrap":
				namespace, err := w.store.kube.CoreV1().Namespaces().Get(t.Context(), w.store.namespace, metav1.GetOptions{})
				require.NoError(t, err)
				namespace.Annotations = map[string]string{initialCertificateIntent: "pending"}
				_, err = w.store.kube.CoreV1().Namespaces().Update(t.Context(), namespace, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "incomplete":
				w.state.Phase = rotationRetire
				data, err := json.Marshal(w.state)
				require.NoError(t, err)
				w.journal.Data[rotationJournalKey] = string(data)
				_, err = w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Update(t.Context(), w.journal, metav1.UpdateOptions{})
				require.NoError(t, err)
			}
			kube, dynamic := w.store.kube.(*kubefake.Clientset), w.store.dynamic.(*dynamicfake.FakeDynamicClient)
			kube.ClearActions()
			dynamic.ClearActions()
			err = w.store.checkAggregationTransitionSnapshot(t.Context(), snapshot)
			if scenario == "unchanged" || scenario == "issuer status" || scenario == "source metadata" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			for _, action := range append(kube.Actions(), dynamic.Actions()...) {
				require.Equal(t, "get", action.GetVerb(), "provider adapter must not obtain observations or mutate")
			}
		})
	}
}
