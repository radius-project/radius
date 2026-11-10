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
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

func newRotationTestStore(t *testing.T) (*rotationStore, *rest.Config, []byte, []byte) {
	t.Helper()
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for isolated CA rotation storage tests.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, kube := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	_, err = kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	old, _, _ := testSigningCA(t, nil, nil, nil, nil)
	next, _, _ := testSigningCA(t, nil, nil, nil, nil)
	_, err = client.Resource(apiServiceResource).Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{
			"name": radiusAPIService,
			"annotations": map[string]any{
				"meta.helm.sh/release-name": radiusReleaseName, "meta.helm.sh/release-namespace": RadiusSystemNamespace,
			},
		},
		"spec": map[string]any{
			"group": "api.ucp.dev", "version": "v1alpha3", "groupPriorityMinimum": int64(1000), "versionPriority": int64(15),
			"service":  map[string]any{"namespace": RadiusSystemNamespace, "name": "ucp"},
			"caBundle": base64.StdEncoding.EncodeToString(old),
		},
	}}, metav1.CreateOptions{FieldManager: FieldManager})
	require.NoError(t, err)
	return &rotationStore{kube: kube, dynamic: client, namespace: RadiusSystemNamespace, release: radiusReleaseName,
		now: time.Now, guardWrite: func(context.Context) error { return nil }}, config, old, next
}

func advanceRotationTest(t *testing.T, store *rotationStore, next rotationPhase) {
	t.Helper()
	journal, state, err := store.read(t.Context())
	require.NoError(t, err)
	require.NoError(t, store.advance(t.Context(), journal, state, next))
	require.NoError(t, store.reconcileTrust(t.Context(), state.ID))
}

func TestRotationStoreLifecycle(t *testing.T) {
	store, _, old, next := newRotationTestStore(t)
	id := uuid.NewString()
	state, err := store.begin(t.Context(), id, next)
	require.NoError(t, err)
	require.Equal(t, old, state.OldCA)
	journal, loaded, err := store.read(t.Context())
	require.NoError(t, err)
	require.Equal(t, state, loaded)
	require.Empty(t, journal.OwnerReferences)
	require.Empty(t, journal.Annotations["meta.helm.sh/release-name"])
	require.ErrorContains(t, store.advance(t.Context(), journal, state, rotationAdopt), "has not published")
	require.NoError(t, store.reconcileTrust(t.Context(), id))
	require.NoError(t, store.reconcileTrust(t.Context(), id), "retry of an applied phase is idempotent")
	require.Error(t, store.advance(t.Context(), journal, state, rotationRetire), "cannot skip consumer/issuance phases")
	_, err = store.begin(t.Context(), uuid.NewString(), next)
	require.Error(t, err, "cannot replace an unfinished transaction")

	for _, phase := range rotationPhases[1:] {
		advanceRotationTest(t, store, phase)
	}
	live, err := store.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	bundle, err := apiServiceBundle(live)
	require.NoError(t, err)
	require.Equal(t, next, bundle)
	require.Equal(t, id+"/complete", live.GetAnnotations()[rotationRevision])

	third, _, _ := testSigningCA(t, nil, nil, nil, nil)
	second, err := store.begin(t.Context(), uuid.NewString(), third)
	require.NoError(t, err, "completed rotations do not freeze later rotations")
	require.Equal(t, next, second.OldCA)
	require.NoError(t, store.reconcileTrust(t.Context(), second.ID))
	require.ErrorContains(t, store.reconcileTrust(t.Context(), id), "transaction or APIService identity changed")
	for _, phase := range rotationPhases[1:] {
		advanceRotationTest(t, store, phase)
	}
}

type rotationRoundTripper func(*http.Request) (*http.Response, error)

func (f rotationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestRotationStoreRejectsDelayedWriteWithoutAdmissionPolicy(t *testing.T) {
	store, config, _, next := newRotationTestStore(t)
	id := uuid.NewString()
	_, err := store.begin(t.Context(), id, next)
	require.NoError(t, err)
	require.NoError(t, store.reconcileTrust(t.Context(), id))

	paused, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(release) }) })
	delayedConfig := rest.CopyConfig(config)
	delayedConfig.WrapTransport = func(base http.RoundTripper) http.RoundTripper {
		return rotationRoundTripper(func(request *http.Request) (*http.Response, error) {
			if request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/apiservices/"+radiusAPIService) {
				close(paused)
				select {
				case <-release:
				case <-request.Context().Done():
					return nil, request.Context().Err()
				}
			}
			return base.RoundTrip(request)
		})
	}
	delayed, err := dynamic.NewForConfig(delayedConfig)
	require.NoError(t, err)
	writer := *store
	writer.dynamic = delayed
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- writer.reconcileTrust(ctx, id) }()
	select {
	case <-paused:
	case <-ctx.Done():
		t.Fatal("writer did not reach its conditional update")
	}
	for _, phase := range rotationPhases[1:] {
		advanceRotationTest(t, store, phase)
	}
	unblock.Do(func() { close(release) })
	require.True(t, apierrors.IsConflict(<-result), "a delayed write must not resurrect the old CA")
	live, err := store.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	bundle, err := apiServiceBundle(live)
	require.NoError(t, err)
	require.Equal(t, next, bundle)
	// Restarting the complete read sequence computes current intent, not the
	// stale overlap with a freshly acquired resourceVersion.
	require.NoError(t, store.reconcileTrust(t.Context(), id))
}

func TestRotationStoreRecoversAmbiguousWrite(t *testing.T) {
	store, config, _, next := newRotationTestStore(t)
	id := uuid.NewString()
	_, err := store.begin(t.Context(), id, next)
	require.NoError(t, err)
	lostConfig := rest.CopyConfig(config)
	lostConfig.WrapTransport = func(base http.RoundTripper) http.RoundTripper {
		return rotationRoundTripper(func(request *http.Request) (*http.Response, error) {
			response, err := base.RoundTrip(request)
			if err == nil && request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/apiservices/"+radiusAPIService) {
				require.NoError(t, response.Body.Close())
				return nil, errors.New("simulated lost response after successful write")
			}
			return response, err
		})
	}
	lost, err := dynamic.NewForConfig(lostConfig)
	require.NoError(t, err)
	writer := *store
	writer.dynamic = lost
	require.Error(t, writer.reconcileTrust(t.Context(), id))
	require.NoError(t, store.reconcileTrust(t.Context(), id))
	advanceRotationTest(t, store, rotationAdopt)
}

func TestRotationStoreRejectsJournalReplay(t *testing.T) {
	store, _, _, next := newRotationTestStore(t)
	id := uuid.NewString()
	_, err := store.begin(t.Context(), id, next)
	require.NoError(t, err)
	require.NoError(t, store.reconcileTrust(t.Context(), id))
	initial, _, err := store.read(t.Context())
	require.NoError(t, err)
	for _, phase := range rotationPhases[1:] {
		advanceRotationTest(t, store, phase)
	}
	maps := store.kube.CoreV1().ConfigMaps(store.namespace)
	_, err = maps.Update(t.Context(), initial, metav1.UpdateOptions{})
	require.True(t, apierrors.IsConflict(err), "stale journal writes must fail")
	current, _, err := store.read(t.Context())
	require.NoError(t, err)
	current.Data = initial.Data
	_, err = maps.Update(t.Context(), current, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, store.reconcileTrust(t.Context(), id), "older than or unrelated")

	require.NoError(t, maps.Delete(t.Context(), rotationJournalName, metav1.DeleteOptions{}))
	_, err = store.begin(t.Context(), uuid.NewString(), initialCA(t, initial))
	require.Error(t, err, "missing journal cannot silently reset the trust history")
	initial.UID, initial.ResourceVersion = "", ""
	_, err = maps.Create(t.Context(), initial, metav1.CreateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, store.reconcileTrust(t.Context(), id), "journal was replaced")
}

func initialCA(t *testing.T, journal *corev1.ConfigMap) []byte {
	t.Helper()
	var state rotationState
	require.NoError(t, json.Unmarshal([]byte(journal.Data[rotationJournalKey]), &state))
	return state.OldCA
}

func TestRotationImmutablePublicTrust(t *testing.T) {
	store, _, old, next := newRotationTestStore(t)
	bundle := append(append([]byte{}, old...), next...)
	overlap, digest, err := store.publishComponentTrust(t.Context(), bundle)
	require.NoError(t, err)
	require.True(t, *overlap.Immutable)
	require.Empty(t, overlap.OwnerReferences)
	require.Empty(t, overlap.Annotations["meta.helm.sh/release-name"])
	retry, repeated, err := store.publishComponentTrust(t.Context(), bundle)
	require.NoError(t, err)
	require.Equal(t, digest, repeated)
	require.Equal(t, overlap.UID, retry.UID)
	require.Equal(t, overlap.ResourceVersion, retry.ResourceVersion)

	changed := overlap.DeepCopy()
	changed.Data["ca.crt"] = string(next)
	_, err = store.kube.CoreV1().ConfigMaps(store.namespace).Update(t.Context(), changed, metav1.UpdateOptions{})
	require.True(t, apierrors.IsInvalid(err), "native immutability must prevent changing mounted trust underneath old processes")
	retired, _, err := store.publishComponentTrust(t.Context(), next)
	require.NoError(t, err)
	require.NotEqual(t, overlap.Name, retired.Name)
	retained, err := store.kube.CoreV1().ConfigMaps(store.namespace).Get(t.Context(), overlap.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, string(bundle), retained.Data["ca.crt"], "publishing retirement must not mutate the overlap still used by old pods")

	retired.Labels[rotationOwner] = "someone-else"
	_, err = store.kube.CoreV1().ConfigMaps(store.namespace).Update(t.Context(), retired, metav1.UpdateOptions{})
	require.NoError(t, err)
	_, _, err = store.publishComponentTrust(t.Context(), next)
	require.ErrorContains(t, err, "ownership")
	_, _, err = store.publishComponentTrust(t.Context(), []byte("not a public certificate"))
	require.Error(t, err)
	third, _, _ := testSigningCA(t, nil, nil, nil, nil)
	_, _, err = store.publishComponentTrust(t.Context(), append(bundle, third...))
	require.Error(t, err, "component overlap must not silently expand to extra trust roots")
}
