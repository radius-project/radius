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
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

func TestObserveTLSStartupProof(t *testing.T) {
	testObserveTLSProof(t, false)
}

func TestObserveAggregationProof(t *testing.T) {
	testObserveTLSProof(t, true)
}

func testObserveTLSProof(t *testing.T, aggregation bool) {
	t.Helper()
	t.Parallel()
	const podUID = "220d6840-2ccf-4121-bdc6-a1f686d2f635"
	const digest = "6134ae3055977f0d025f1c2ada38ebdcad54514560ee2819b32a4a33e03f3a9f"
	now := time.Now().UTC()
	target := TLSProofTarget{Namespace: "radius-system", PodName: "bicep-de-test", PodUID: podUID,
		ContainerName: "de", Port: 6443, TrustBundleSHA256: digest, LeafSHA256: strings.Repeat("2", 64)}
	evidence := mtls.StartupEvidence{SchemaVersion: 1, Ready: true, ProofKind: "startup-tls-initialized",
		PodUID: podUID, BootID: "ac1a87e6-d265-4da5-8e22-56591b425ca0", InitializedAt: now.Add(-time.Minute),
		TrustBundleSHA256: digest, CASHA256: []string{strings.Repeat("0", 64), strings.Repeat("0", 63) + "1"},
		LeafSHA256: target.LeafSHA256, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	path := mtls.StartupProofPath
	if aggregation {
		target.ContainerName = "ucp"
		evidence.ProofKind = "aggregation-serving-tls-initialized"
		path = mtls.AggregationProofPath
	}
	body, err := json.Marshal(evidence)
	require.NoError(t, err)
	for _, scenario := range []string{"success", "replaced before", "replaced after", "restarted", "terminating",
		"not running", "missing started time", "proof predates container", "pod read forbidden", "pod recheck forbidden", "port-forward forbidden", "unready", "redirect", "oversize", "duplicate", "missing", "unknown", "null", "trailing", "wrong trust", "wrong leaf", "wrong proof kind", "canceled body", "deadline body"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			var gets atomic.Int32
			var redirected atomic.Bool
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if scenario == "deadline body" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 300*time.Millisecond)
				defer deadlineCancel()
			}
			redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirected.Store(true)
				w.Write(body)
			}))
			defer redirect.Close()
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.Header.Get("Authorization"), "never forward Kubernetes credentials to the workload")
				require.Empty(t, r.Header.Get("Cookie"))
				require.Empty(t, r.Header.Get("Impersonate-User"))
				if r.URL.Path != path {
					http.Error(w, "wrong target", http.StatusBadRequest)
					return
				}
				switch scenario {
				case "wrong proof kind":
					wrong := evidence
					if aggregation {
						wrong.ProofKind = "startup-tls-initialized"
					} else {
						wrong.ProofKind = "aggregation-serving-tls-initialized"
					}
					json.NewEncoder(w).Encode(wrong)
				case "canceled body", "deadline body":
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					if scenario == "canceled body" {
						cancel()
					}
					<-r.Context().Done()
				case "unready":
					http.Error(w, `{"schemaVersion":1,"ready":false}`, http.StatusServiceUnavailable)
				case "redirect":
					http.Redirect(w, r, redirect.URL, http.StatusFound)
				case "oversize":
					w.Write([]byte(strings.Repeat(" ", 8193)))
				case "duplicate":
					w.Write([]byte(strings.Replace(string(body), `"ready":true`, `"ready":false,"ready":true`, 1)))
				case "missing":
					w.Write([]byte(strings.Replace(string(body), `,"peerHandshakeVerified":false`, "", 1)))
				case "unknown":
					w.Write([]byte(strings.Replace(string(body), `"ready":true`, `"ready":true,"privateKey":"sentinel"`, 1)))
				case "null":
					w.Write([]byte(strings.Replace(string(body), `"peerHandshakeVerified":false`, `"peerHandshakeVerified":null`, 1)))
				case "trailing":
					w.Write(append(body, []byte("{}")...))
				default:
					w.Write(body)
				}
			}))
			defer receiver.Close()
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer proof-observer" {
					http.Error(w, "authentication required", http.StatusUnauthorized)
					return
				}
				require.Equal(t, "proof-reader", r.Header.Get("Impersonate-User"))
				if r.URL.Path == "/api/v1/namespaces/radius-system/pods/bicep-de-test/portforward" {
					if scenario == "port-forward forbidden" {
						http.Error(w, "denied private upstream response", http.StatusForbidden)
						return
					}
					servePodProofForward(t, w, r, receiver.Listener.Addr().String(), target.Port)
					return
				}
				require.Equal(t, "/api/v1/namespaces/radius-system/pods/bicep-de-test", r.URL.Path)
				require.Equal(t, http.MethodGet, r.Method)
				count := gets.Add(1)
				if scenario == "pod read forbidden" || scenario == "pod recheck forbidden" && count > 1 {
					http.Error(w, "pod read forbidden", http.StatusForbidden)
					return
				}
				pod := corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
					ObjectMeta: metav1.ObjectMeta{Name: target.PodName, Namespace: target.Namespace, UID: types.UID(podUID)},
					Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
						{Name: target.ContainerName, ContainerID: "containerd://one", State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-time.Minute))}}},
					}}}
				if scenario == "replaced before" || (scenario == "replaced after" && count > 1) {
					pod.UID = "different-pod"
				}
				if scenario == "restarted" && count > 1 {
					pod.Status.ContainerStatuses[0].ContainerID = "containerd://two"
					pod.Status.ContainerStatuses[0].RestartCount = 1
				}
				if scenario == "terminating" {
					at := metav1.NewTime(now)
					pod.DeletionTimestamp = &at
				}
				if scenario == "not running" {
					pod.Status.ContainerStatuses[0].State.Running = nil
				}
				if scenario == "missing started time" {
					pod.Status.ContainerStatuses[0].State.Running.StartedAt = metav1.Time{}
				}
				if scenario == "proof predates container" {
					pod.Status.ContainerStatuses[0].State.Running.StartedAt = metav1.NewTime(now)
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(pod)
			}))
			defer server.Close()
			expected := target
			if scenario == "wrong trust" {
				expected.TrustBundleSHA256 = strings.Repeat("3", 64)
			}
			if scenario == "wrong leaf" {
				expected.LeafSHA256 = strings.Repeat("3", 64)
			}
			config := &rest.Config{Host: server.URL, BearerToken: "proof-observer",
				Impersonate:     rest.ImpersonationConfig{UserName: "proof-reader"},
				TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})}}
			var proof PodTLSProof
			var err error
			if aggregation {
				var serving PodAggregationProof
				serving, err = ObserveAggregationProof(ctx, config, expected)
				proof = PodTLSProof{Evidence: mtls.StartupEvidence(serving.Evidence), ContainerID: serving.ContainerID, RestartCount: serving.RestartCount}
			} else {
				proof, err = ObserveTLSStartupProof(ctx, config, expected)
			}
			if scenario == "success" {
				require.NoError(t, err)
				require.Equal(t, "containerd://one", proof.ContainerID)
				require.Equal(t, evidence.BootID, proof.Evidence.BootID)
				require.False(t, proof.Evidence.PeerHandshakeVerified)
				require.EqualValues(t, 2, gets.Load())
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private upstream response")
				require.Zero(t, proof)
				if scenario == "canceled body" {
					require.ErrorIs(t, err, context.Canceled)
				}
				if scenario == "deadline body" {
					require.ErrorIs(t, err, context.DeadlineExceeded)
				}
			}
			require.False(t, redirected.Load(), "never follow proof endpoint redirects")
		})
	}
}
