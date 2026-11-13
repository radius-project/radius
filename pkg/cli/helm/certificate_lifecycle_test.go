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
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func lifecycleFixture(t *testing.T) (certificateLifecycle, *corev1.ConfigMap) {
	t.Helper()
	state := certificateLifecycle{SchemaVersion: 4, ID: uuid.NewString(), Namespace: RadiusSystemNamespace,
		Release: radiusReleaseName, Provider: "radius", Mode: "selfSigned", Phase: rotationPublish,
		CreatedAt: time.Now().Add(-time.Minute), APIServiceUID: types.UID(uuid.NewString()),
		Planes: map[string]certificateLifecyclePlane{}}
	for _, name := range []string{"internal", "aggregation"} {
		old, _, _ := testSigningCA(t, nil, nil, nil, nil)
		next, _, nextCertificate := testSigningCA(t, nil, nil, nil, nil)
		bundle := append(bytes.Clone(old), next...)
		kind, source := "ConfigMap", "radius-mtls-ca"
		if name == "aggregation" {
			kind, source = "Secret", "radius-apiserver-trust"
		}
		state.Planes[name] = certificateLifecyclePlane{Affected: true, OldCA: old, NewCA: next,
			Issuer: certificateIssuerReference{Group: "cert-manager.io", Kind: "Issuer", Name: "target-" + name},
			Signer: &certificateObjectPin{Name: "target-" + name, UID: types.UID(uuid.NewString()),
				ResourceVersion: "1", SHA256: fmt.Sprintf("%x", sha256.Sum256(nextCertificate.Raw))},
			Source: certificateTrustProjection{Kind: kind, Bundle: bundle,
				certificateObjectPin: certificateObjectPin{Name: source, UID: types.UID(uuid.NewString()),
					ResourceVersion: "2", SHA256: fmt.Sprintf("%x", sha256.Sum256(bundle))}}}
	}
	journal := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: rotationJournalName,
		Namespace: RadiusSystemNamespace, UID: types.UID(uuid.NewString()), ResourceVersion: "3",
		Labels: map[string]string{rotationOwner: radiusReleaseName}}}
	return state, journal
}

func TestCertificateLifecycleIndependentPlanes(t *testing.T) {
	t.Parallel()
	state, journal := lifecycleFixture(t)
	require.NoError(t, state.validate(time.Now()))
	aggregate := state.Planes["aggregation"]
	aggregate.Affected = false
	aggregate.NewCA, aggregate.Source.Bundle = aggregate.OldCA, aggregate.OldCA
	anchors, err := parseCertificateBundlePEM(aggregate.OldCA)
	require.NoError(t, err)
	aggregate.Signer.SHA256 = fmt.Sprintf("%x", sha256.Sum256(anchors[0].Raw))
	aggregate.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(aggregate.OldCA))
	state.Planes["aggregation"] = aggregate
	state.APIServiceUID = ""
	require.NoError(t, state.validate(time.Now()), "internal-only rotation does not require or alter APIService identity")
	for _, phase := range []rotationPhase{rotationRetire, rotationComplete} {
		state.Phase = phase
		internal := state.Planes["internal"]
		internal.Source.Bundle = internal.NewCA
		internal.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(internal.NewCA))
		state.Planes["internal"] = internal
		data, err := json.Marshal(state)
		require.NoError(t, err)
		journal.Data = map[string]string{rotationJournalKey: string(data)}
		decoded, err := decodeCertificateLifecycle(journal, state.Namespace, state.Release, time.Now())
		require.NoError(t, err)
		require.Equal(t, aggregate, decoded.Planes["aggregation"])
	}
}

func TestCertificateLifecycleRejectsLegacyAndUnobservedPins(t *testing.T) {
	t.Parallel()
	state, journal := lifecycleFixture(t)
	for _, change := range []func(*certificateLifecycle){
		func(s *certificateLifecycle) { s.SchemaVersion = 3 },
		func(s *certificateLifecycle) { s.Provider = "external" },
		func(s *certificateLifecycle) { s.Phase = rotationRetire },
		func(s *certificateLifecycle) {
			p := s.Planes["internal"]
			p.Source.UID = ""
			s.Planes["internal"] = p
		},
		func(s *certificateLifecycle) {
			p := s.Planes["internal"]
			p.NewCA = p.OldCA
			s.Planes["internal"] = p
		},
		func(s *certificateLifecycle) {
			s.Consumers = []certificateConsumerPin{{WorkloadUID: "w", WorkloadGeneration: 1, PodUID: "p", PodName: "controller",
				ServiceAccount: "controller", ServiceAccountUID: "sa", TrustOnly: true, Driver: "invented-client-leaf"}}
		},
	} {
		copy := state
		copy.Planes = map[string]certificateLifecyclePlane{}
		for name, plane := range state.Planes {
			copy.Planes[name] = plane
		}
		change(&copy)
		data, err := json.Marshal(copy)
		require.NoError(t, err)
		journal.Data = map[string]string{rotationJournalKey: string(data)}
		_, err = decodeCertificateLifecycle(journal, state.Namespace, state.Release, time.Now())
		require.Error(t, err)
	}
}

func TestExactCertificateSet(t *testing.T) {
	t.Parallel()
	a, _, _ := testSigningCA(t, nil, nil, nil, nil)
	b, _, _ := testSigningCA(t, nil, nil, nil, nil)
	both := append(bytes.Clone(a), b...)
	require.NoError(t, exactCertificateSet(append(bytes.Clone(b), a...), both, time.Now()))
	for _, actual := range [][]byte{a, b, append(bytes.Clone(both), a...), append(bytes.Clone(both), []byte("junk")...)} {
		require.Error(t, exactCertificateSet(actual, both, time.Now()))
	}
}

func TestCertificateLifecycleHistoricalAnchorPins(t *testing.T) {
	t.Parallel()
	state, _ := lifecycleFixture(t)
	plane := state.Planes["internal"]
	plane.Source.Bundle = plane.NewCA
	plane.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(plane.NewCA))
	require.NoError(t, plane.validate("internal", rotationComplete, time.Now()))

	duplicate := plane
	duplicate.OldCA = append(bytes.Clone(plane.OldCA), plane.OldCA...)
	require.ErrorContains(t, duplicate.validate("internal", rotationComplete, time.Now()), "exactly one previous")

	signer := *plane.Signer
	signer.SHA256 = fmt.Sprintf("%x", sha256.Sum256(plane.NewCA))
	plane.Signer = &signer
	require.ErrorContains(t, plane.validate("internal", rotationComplete, time.Now()), "public DER pin differs")
}

func TestLifecycleConsumerPinsRequireObservedIdentities(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"trust only", "CSI receiver", "missing SA UID", "missing generation", "missing driver UID"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			state, _ := lifecycleFixture(t)
			consumer := certificateConsumerPin{WorkloadUID: "workload", WorkloadGeneration: 1,
				PodName: "ucp-pod", PodUID: "pod", ServiceAccount: "ucp", ServiceAccountUID: "account", TrustOnly: true}
			switch scenario {
			case "CSI receiver", "missing driver UID":
				consumer.TrustOnly = false
				consumer.Driver, consumer.DriverUID = SPIFFECSIDriverNameDefault, "driver"
				consumer.ServerSPIFFEID = "spiffe://radius.internal/ns/radius-system/sa/ucp"
				if scenario == "missing driver UID" {
					consumer.DriverUID = ""
				}
			case "missing SA UID":
				consumer.ServiceAccountUID = ""
			case "missing generation":
				consumer.WorkloadGeneration = 0
			}
			state.Consumers = []certificateConsumerPin{consumer}
			err := state.validate(time.Now())
			if scenario == "trust only" || scenario == "CSI receiver" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestCertificateRotationPlaneSelection(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		provider, scope string
		want            []string
	}{
		{"radius", "", []string{"internal", "aggregation"}},
		{"external", "", []string{"aggregation"}},
		{"radius", "internal", []string{"internal"}},
		{"external", "internal", nil},
	} {
		actual, err := certificateRotationPlanes(tt.provider, tt.scope)
		if tt.want == nil {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, tt.want, actual)
		}
	}
	proof, err := ObserveTLSStartupProof(t.Context(), nil, TLSProofTarget{})
	require.ErrorIs(t, err, errCertificateObserverUnavailable)
	require.Zero(t, proof)
	aggregation, err := ObserveAggregationProof(t.Context(), nil, TLSProofTarget{})
	require.ErrorIs(t, err, errCertificateObserverUnavailable)
	require.Zero(t, aggregation)
}

func TestLifecycleProjectionRevisionAndIdentity(t *testing.T) {
	t.Parallel()
	for _, phase := range []rotationPhase{rotationPublish, rotationComplete} {
		for _, scenario := range []string{"unchanged", "metadata update", "replacement", "changed bundle", "foreign journal"} {
			t.Run(string(phase)+"/"+scenario, func(t *testing.T) {
				t.Parallel()
				state, journal := lifecycleFixture(t)
				state.Phase = phase
				plane := state.Planes["aggregation"]
				if phase == rotationComplete {
					plane.Source.Bundle = plane.NewCA
				}
				source := permanentPublicTrustSecret(plane.Source.Bundle)
				source.UID, source.ResourceVersion = plane.Source.UID, plane.Source.ResourceVersion
				source.Annotations[rotationJournalUID] = string(journal.UID)
				source.Annotations[rotationRevision] = state.ID + "/" + string(phase)
				switch scenario {
				case "metadata update":
					source.ResourceVersion = "later-version"
					source.Annotations["unrelated"] = "unchanged-trust"
				case "replacement":
					source.UID = "replacement"
				case "changed bundle":
					source.Data[caCertificateKey] = plane.OldCA
				case "foreign journal":
					source.Annotations[rotationJournalUID] = "foreign"
				}
				store := &rotationStore{kube: kubefake.NewClientset(source)}
				err := store.checkLifecycleProjection(t.Context(), journal, state, "aggregation", plane)
				if scenario == "unchanged" || scenario == "metadata update" && phase == rotationComplete {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			})
		}
	}
}
