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
	"crypto/x509"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// These are local source-bound data fixtures, not a CLI credential or installed
// endpoint acceptance test. No HTTP observer is enabled or token obtained.
func TestOutboundTrustEvidenceSourceBoundFixture(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{
		"valid", "legacy", "unknown leaf", "null handshake", "missing handshake", "duplicate ready",
		"wrong pod", "wrong UCP identity", "wrong bounds", "restarted", "replaced", "predates container",
		"oversized", "expired", "duplicate anchors", "changed namespace",
	} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			old, _, _ := testSigningCA(t, nil, nil, nil, nil)
			next, _, _ := testSigningCA(t, nil, func(certificate *x509.Certificate) {
				certificate.NotBefore = time.Now().Add(-30 * time.Minute)
				certificate.NotAfter = time.Now().Add(24 * time.Hour)
			}, nil, nil)
			bundle := append(append([]byte{}, old...), next...)
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, caCertificateKey), bundle, 0600))
			source, err := mtls.NewTrustSource(directory)
			require.NoError(t, err)
			podUID := uuid.NewString()
			serverID := mtls.SPIFFEID("radius.internal", RadiusSystemNamespace, "ucp")
			digest, _, err := mtls.TrustBundleDigest(bundle, time.Now())
			require.NoError(t, err)
			proof, err := mtls.NewTrustProof(source, podUID, digest, serverID)
			require.NoError(t, err)
			evidence, err := proof.Snapshot()
			require.NoError(t, err)
			before := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "bicep-de-fixture", Namespace: RadiusSystemNamespace, UID: types.UID(podUID)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
					Name: "bicep-de", ContainerID: "containerd://fixture", Ready: true,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{
						StartedAt: metav1.NewTime(evidence.InitializedAt.Add(-time.Minute))}},
				}}}}
			after := before.DeepCopy()
			target := TLSProofTarget{Namespace: before.Namespace, PodName: before.Name, PodUID: before.UID,
				ContainerName: "bicep-de", Port: 6443, TrustBundleSHA256: digest}
			now := time.Now()
			switch scenario {
			case "legacy":
				evidence.SchemaVersion = 1
			case "wrong pod":
				evidence.PodUID = uuid.NewString()
			case "wrong UCP identity":
				evidence.ExpectedServerSPIFFEID = mtls.SPIFFEID("radius.internal", RadiusSystemNamespace, "controller")
			case "wrong bounds":
				evidence.NotAfter = evidence.NotAfter.Add(time.Hour)
			case "restarted":
				after.Status.ContainerStatuses[0].RestartCount++
			case "replaced":
				after.UID = types.UID(uuid.NewString())
			case "predates container":
				before.Status.ContainerStatuses[0].State.Running.StartedAt = metav1.NewTime(evidence.InitializedAt.Add(time.Second))
				after = before.DeepCopy()
			case "expired":
				now = evidence.NotAfter.Add(time.Second)
			case "duplicate anchors":
				bundle = append(bundle, old...)
			case "changed namespace":
				after.Namespace = "other"
			}
			data, err := json.Marshal(evidence)
			require.NoError(t, err)
			switch scenario {
			case "unknown leaf", "null handshake", "missing handshake":
				var fields map[string]any
				require.NoError(t, json.Unmarshal(data, &fields))
				switch scenario {
				case "unknown leaf":
					fields["leafSha256"] = digest
				case "null handshake":
					fields["peerHandshakeVerified"] = nil
				case "missing handshake":
					delete(fields, "peerHandshakeVerified")
				}
				data, err = json.Marshal(fields)
				require.NoError(t, err)
			case "duplicate ready":
				data = bytes.Replace(data, []byte(`"ready":true`), []byte(`"ready":true,"ready":true`), 1)
			case "oversized":
				data = append(bytes.Repeat([]byte(" "), 8192), data...)
			}
			observed, err := validateOutboundTrustEvidence(data, target, bundle, serverID, before, after, now)
			if scenario == "valid" {
				require.NoError(t, err)
				require.Equal(t, evidence, observed.Evidence)
				require.Equal(t, "containerd://fixture", observed.ContainerID)
				require.False(t, observed.Evidence.PeerHandshakeVerified)
			} else {
				require.Error(t, err)
				require.Empty(t, observed)
			}
		})
	}
}
