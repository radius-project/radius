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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func bootstrapStoreFixture(t *testing.T) (*rotationStore, aggregationBootstrapState) {
	t.Helper()
	kube := kubefake.NewSimpleClientset()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	version := 0
	metadata := func(action k8stesting.Action) (bool, runtime.Object, error) {
		var object runtime.Object
		switch action := action.(type) {
		case k8stesting.CreateAction:
			object = action.GetObject()
		case k8stesting.UpdateAction:
			object = action.GetObject()
		default:
			return false, nil, nil
		}
		accessor, err := meta.Accessor(object)
		require.NoError(t, err)
		if accessor.GetUID() == "" {
			accessor.SetUID(types.UID(uuid.NewString()))
		}
		version++
		accessor.SetResourceVersion(fmt.Sprint(version))
		return false, nil, nil
	}
	kube.PrependReactor("*", "*", metadata)
	client.PrependReactor("*", "*", metadata)
	s := &rotationStore{kube: kube, dynamic: client, namespace: RadiusSystemNamespace, release: radiusReleaseName,
		now: time.Now, guardWrite: func(context.Context) error { return nil }}
	state := aggregationBootstrapState{Version: 1, ID: uuid.NewString(), Namespace: s.namespace, Release: s.release,
		Phase: bootstrapPreparing, CreatedAt: time.Now().UTC(), Scope: rotationComponentsAndAggregation,
		Mode: "selfSigned", IntentSHA256: strings.Repeat("a", 64), ChartSHA256: strings.Repeat("b", 64), Resources: map[string]aggregationBootstrapPin{}}
	s.activeBootstrap = state.ID
	return s, state
}

func TestBootstrapStrictIntent(t *testing.T) {
	t.Parallel()
	s, state := bootstrapStoreFixture(t)
	require.NoError(t, state.validate(s.now()))
	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "null")
	for _, invalid := range []string{
		`null`, string(encoded) + `{}`, strings.Replace(string(encoded), `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(string(encoded), `"version":1`, `"version":1,"ver\u0073ion":1`, 1),
		strings.Replace(string(encoded), `"resources":{}`, `"resources":null`, 1),
		strings.Replace(string(encoded), `"resources":{}`, `"resources":{"x":{"uid":"u","uid":"u"}}`, 1),
		strings.Replace(string(encoded), `"version":1`, `"version":1,"unknown":true`, 1),
	} {
		_, err := decodeAggregationBootstrap(invalid)
		require.Error(t, err, invalid)
	}
	journal, persisted, err := s.beginBootstrap(t.Context(), state)
	require.NoError(t, err)
	require.Empty(t, persisted.CA)
	require.Empty(t, persisted.Resources)
	retry := state
	retry.ID = uuid.NewString()
	retried, resumed, err := s.beginBootstrap(t.Context(), retry)
	require.NoError(t, err)
	require.Equal(t, journal.UID, retried.UID)
	require.Equal(t, state.ID, resumed.ID)
	retry.IntentSHA256 = strings.Repeat("b", 64)
	_, _, err = s.beginBootstrap(t.Context(), retry)
	require.ErrorContains(t, err, "retry differs")
	next := persisted
	next.Phase = bootstrapComplete
	_, err = s.updateBootstrap(t.Context(), journal, persisted, next)
	require.ErrorContains(t, err, "phase transition")
}

func TestBootstrapCreateAndRecordRecovery(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"normal", "lost journal write", "foreign", "replaced", "changed spec", "lock changed"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			s, state := bootstrapStoreFixture(t)
			ctx := t.Context()
			_, _, err := s.beginBootstrap(ctx, state)
			require.NoError(t, err)
			desired := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
				"metadata": map[string]any{"name": "radius-selfsigned-bootstrap", "namespace": s.namespace},
				"spec":     map[string]any{"selfSigned": map[string]any{}},
			}}
			kube := s.kube.(*kubefake.Clientset)
			client := s.dynamic.(*dynamicfake.FakeDynamicClient)
			if scenario == "foreign" {
				foreign := desired.DeepCopy()
				foreign.SetUID("foreign")
				foreign.SetResourceVersion("1")
				require.NoError(t, client.Tracker().Add(foreign))
			}
			if scenario == "lock changed" {
				s.guardWrite = func(context.Context) error { return errors.New("identity lock changed") }
			}
			lost := scenario == "lost journal write"
			kube.PrependReactor("update", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
				if lost {
					return true, nil, errors.New("simulated journal failure")
				}
				return false, nil, nil
			})
			err = s.ensureBootstrapResource(ctx, state.ID, rotationIssuerResource, desired, true)
			switch scenario {
			case "foreign", "lock changed":
				require.Error(t, err)
				return
			case "lost journal write":
				require.ErrorContains(t, err, "journal failure")
				lost = false
				_, prior, err := s.readBootstrap(ctx)
				require.NoError(t, err)
				require.Empty(t, prior.Resources)
				require.NoError(t, s.ensureBootstrapResource(ctx, state.ID, rotationIssuerResource, desired, true))
			default:
				require.NoError(t, err)
			}
			_, pinned, err := s.readBootstrap(ctx)
			require.NoError(t, err)
			require.Len(t, pinned.Resources, 1)
			object, err := client.Resource(rotationIssuerResource).Namespace(s.namespace).Get(ctx, desired.GetName(), metav1.GetOptions{})
			require.NoError(t, err)
			switch scenario {
			case "replaced":
				object.SetUID("replaced")
			case "changed spec":
				object.Object["spec"] = map[string]any{"ca": map[string]any{"secretName": "foreign"}}
			}
			require.NoError(t, client.Tracker().Update(rotationIssuerResource, object, s.namespace))
			err = s.ensureBootstrapResource(ctx, state.ID, rotationIssuerResource, desired, true)
			if scenario == "replaced" || scenario == "changed spec" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBootstrapCompletedLifecycleHasNoInventedPredecessor(t *testing.T) {
	t.Parallel()
	for _, aggregationOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(aggregationOnly), func(t *testing.T) {
			t.Parallel()
			_, state := aggregationExecutionFixture(t, aggregationOnly)
			state.SchemaVersion, state.Bootstrap, state.Phase = 3, true, rotationComplete
			state.OldCA, state.Plan.OldComponentCA = nil, nil
			require.NoError(t, state.validate(time.Now()))
			data, err := json.Marshal(state)
			require.NoError(t, err)
			decoded, err := decodeRotationState(string(data))
			require.NoError(t, err)
			require.NoError(t, decoded.validate(time.Now()))
			state.Phase = rotationPublish
			require.Error(t, state.validate(time.Now()), "bootstrap completion must not authorize synthetic rotation phases")
		})
	}
}

func TestBootstrapNamespaceIntentRecovery(t *testing.T) {
	t.Parallel()
	s, requested := bootstrapStoreFixture(t)
	impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return s, nil }}
	b := &aggregationBootstrapInstaller{store: s, impl: impl}
	recorded, err := b.ensureNamespace(t.Context(), requested)
	require.NoError(t, err)
	require.Equal(t, requested.ID, recorded.ID)
	retry := requested
	retry.ID = uuid.NewString()
	recovered, err := b.ensureNamespace(t.Context(), retry)
	require.NoError(t, err)
	require.Equal(t, requested.ID, recovered.ID, "namespace create-success/journal-write loss preserves original intent")
	retry.IntentSHA256 = strings.Repeat("c", 64)
	_, err = b.ensureNamespace(t.Context(), retry)
	require.ErrorContains(t, err, "different bootstrap intent")
	require.ErrorContains(t, impl.guardCertificateRotation(t.Context(), "", s.namespace, "uninstall", nil), "legacy aggregation bootstrap")
	_, err = impl.prepareAggregationBootstrap(t.Context(), NewDefaultClusterOptions(), &chart.Chart{}, nil,
		map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": false, "dryRun": false}}}, "", nil, nil)
	require.ErrorContains(t, err, "cannot be bypassed")
	s.activeBootstrap = ""
	require.ErrorContains(t, s.checkWrite(t.Context()), "finish the aggregation bootstrap")
	s.activeBootstrap = requested.ID
	require.NoError(t, s.checkWrite(t.Context()))
	_, _, err = s.beginBootstrap(t.Context(), recovered)
	require.NoError(t, err)
}
