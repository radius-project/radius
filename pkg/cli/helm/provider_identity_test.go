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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/httpstream"
	streamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	kubetesting "k8s.io/client-go/testing"
)

func identityTargetFixture(component string) (externalIdentityTarget, *corev1.Pod) {
	target := externalIdentityTarget{Component: component, TLSProofTarget: TLSProofTarget{
		Namespace: "radius-system", PodName: component + "-one", PodUID: types.UID(uuid.NewString()),
		ContainerName: component, Port: 9444,
	}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: target.PodName, Namespace: target.Namespace, UID: target.PodUID},
		Spec:       corev1.PodSpec{ServiceAccountName: component},
		Status: corev1.PodStatus{Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: component, Ready: true, ContainerID: "containerd://original",
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{
					StartedAt: metav1.NewTime(time.Now().Add(-time.Minute))}}}},
		},
	}
	return target, pod
}

func TestExternalIdentityObserver(t *testing.T) {
	t.Parallel()
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		for _, component := range []string{"applications-rp", "dynamic-rp", "controller", "bicep-de"} {
			for _, scenario := range []string{"valid", "stale", "future", "wrong mode", "wrong caller", "wrong namespace",
				"wrong pod", "wrong challenge", "old upstream pod", "old upstream process", "missing upstream",
				"duplicate", "nested duplicate", "null", "wrong casing", "oversize", "redirect", "forbidden",
				"target restarted", "upstream restarted", "upstream replaced", "not ready", "token denied",
				"expired token", "wrong token audience", "wrong token subject", "canceled"} {
				t.Run(string(mode)+"/"+component+"/"+scenario, func(t *testing.T) {
					t.Parallel()
					target, pod := identityTargetFixture(component)
					ucp, ucpPod := identityTargetFixture("ucp")
					if scenario == "not ready" {
						pod.Status.ContainerStatuses[0].Ready = false
					}
					kube := fake.NewClientset(pod, ucpPod)
					key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
					require.NoError(t, err)
					var minted atomic.Int32
					kube.PrependReactor("create", "serviceaccounts", func(action kubetesting.Action) (bool, runtime.Object, error) {
						if scenario == "token denied" {
							return true, nil, errors.New("private upstream error")
						}
						minted.Add(1)
						create := action.(kubetesting.CreateAction)
						request := create.GetObject().(*authenticationv1.TokenRequest).DeepCopy()
						require.Equal(t, "token", create.GetSubresource())
						require.Equal(t, target.Namespace, create.GetNamespace())
						require.EqualValues(t, 600, *request.Spec.ExpirationSeconds)
						bound := request.Spec.BoundObjectRef
						require.Equal(t, "Pod", bound.Kind)
						require.Equal(t, "v1", bound.APIVersion)
						caller := ucp
						if bound.UID == target.PodUID {
							caller = target
						}
						require.Equal(t, caller.PodUID, bound.UID)
						require.Equal(t, caller.PodName, bound.Name)
						claims := jwt.RegisteredClaims{Subject: "system:serviceaccount:" + target.Namespace + ":" + caller.Component,
							Audience: request.Spec.Audiences, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}
						if scenario == "wrong token audience" {
							claims.Audience = jwt.ClaimStrings{"other.radius-system.svc"}
						}
						if scenario == "wrong token subject" {
							claims.Subject = "system:serviceaccount:radius-system:unrelated"
						}
						signed, err := jwt.NewWithClaims(jwt.SigningMethodES256, claims).SignedString(key)
						require.NoError(t, err)
						request.Status.Token = signed
						request.Status.ExpirationTimestamp = metav1.NewTime(time.Now().Add(time.Minute))
						if scenario == "expired token" {
							request.Status.ExpirationTimestamp = metav1.NewTime(time.Now().Add(-time.Minute))
						}
						return true, request, nil
					})
					boot, ucpBoot := uuid.NewString(), uuid.NewString()
					var redirected atomic.Bool
					redirect := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						redirected.Store(true)
					}))
					defer redirect.Close()
					serve := func(expected externalIdentityTarget, ownBoot, caller string) http.HandlerFunc {
						return func(w http.ResponseWriter, r *http.Request) {
							_, err := jwt.ParseWithClaims(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
								&jwt.RegisteredClaims{}, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
								jwt.WithValidMethods([]string{"ES256"}), jwt.WithAudience(expected.Component+"."+target.Namespace+".svc"),
								jwt.WithSubject("system:serviceaccount:"+target.Namespace+":"+caller), jwt.WithExpirationRequired())
							if err != nil {
								w.WriteHeader(http.StatusUnauthorized)
								return
							}
							base := mtls.InternalIdentityReceipt{SchemaVersion: 1, Provider: "external", Mode: mode,
								Component: expected.Component, Namespace: expected.Namespace, PodUID: string(expected.PodUID),
								BootID: ownBoot, Caller: "system:serviceaccount:" + expected.Namespace + ":" + caller,
								Challenge: r.URL.Query().Get("challenge"), ObservedAt: time.Now().UTC()}
							evidence := externalIdentityEvidence{InternalIdentityReceipt: base}
							if expected.Component == "ucp" {
								if scenario == "old upstream process" {
									evidence.BootID = uuid.NewString()
								}
								require.NoError(t, json.NewEncoder(w).Encode(evidence))
								return
							}
							upstream := base
							upstream.Component, upstream.PodUID, upstream.BootID = "ucp", string(ucp.PodUID), ucpBoot
							upstream.Caller = "system:serviceaccount:" + target.Namespace + ":" + target.Component
							evidence.Upstream = &upstream
							switch scenario {
							case "stale":
								evidence.ObservedAt = time.Now().Add(-time.Minute).UTC()
							case "future":
								evidence.ObservedAt = time.Now().Add(time.Minute).UTC()
							case "wrong mode":
								evidence.Mode = authz.ModeOff
							case "wrong caller":
								evidence.Caller = "system:serviceaccount:radius-system:controller"
							case "wrong namespace":
								evidence.Namespace = "other"
							case "wrong pod":
								evidence.PodUID = uuid.NewString()
							case "wrong challenge":
								evidence.Challenge = uuid.NewString()
							case "old upstream pod":
								evidence.Upstream.PodUID = uuid.NewString()
							case "missing upstream":
								evidence.Upstream = nil
							case "target restarted", "upstream restarted", "upstream replaced":
								changed := pod.DeepCopy()
								if scenario != "target restarted" {
									changed = ucpPod.DeepCopy()
								}
								changed.Status.ContainerStatuses[0].RestartCount++
								if scenario == "upstream replaced" {
									changed.UID = types.UID(uuid.NewString())
								}
								_, err := kube.CoreV1().Pods(changed.Namespace).Update(r.Context(), changed, metav1.UpdateOptions{})
								require.NoError(t, err)
							case "redirect":
								http.Redirect(w, r, redirect.URL, http.StatusFound)
								return
							case "forbidden":
								w.WriteHeader(http.StatusForbidden)
								return
							case "oversize":
								_, err := io.WriteString(w, strings.Repeat("x", 8193))
								require.NoError(t, err)
								return
							}
							data, err := json.Marshal(evidence)
							require.NoError(t, err)
							body := string(data)
							switch scenario {
							case "duplicate":
								body = strings.Replace(body, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1)
							case "nested duplicate":
								body = strings.Replace(body, `"upstream":{`, `"upstream":{"mode":"off",`, 1)
							case "null":
								body = strings.Replace(body, `"provider":"external"`, `"provider":null`, 1)
							case "wrong casing":
								body = strings.Replace(body, `"podUID"`, `"podUid"`, 1)
							}
							_, err = io.WriteString(w, body)
							require.NoError(t, err)
						}
					}
					server := httptest.NewServer(serve(target, boot, "ucp"))
					defer server.Close()
					upstream := httptest.NewServer(serve(ucp, ucpBoot, component))
					defer upstream.Close()
					observer := externalIdentityObserver{kube: kube, open: func(_ context.Context, expected externalIdentityTarget) (string, func(), error) {
						if expected.Component == "ucp" {
							return upstream.URL, func() {}, nil
						}
						return server.URL, func() {}, nil
					}}
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					if scenario == "canceled" {
						cancel()
					}
					evidence, err := observer.observe(ctx, mode, target, ucp, map[types.UID]externalIdentityTarget{ucp.PodUID: ucp})
					if scenario == "valid" {
						require.NoError(t, err)
						require.Equal(t, boot, evidence.BootID)
						require.Equal(t, ucpBoot, evidence.Upstream.BootID)
						require.EqualValues(t, 2, minted.Load())
					} else {
						require.Error(t, err)
						require.NotContains(t, err.Error(), "private upstream")
					}
					require.False(t, redirected.Load())
				})
			}
		}
	}
}

func TestExternalIdentityObserverUCP(t *testing.T) {
	t.Parallel()
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce, authz.ModeOff} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			target, pod := identityTargetFixture("ucp")
			caller, callerPod := identityTargetFixture("bicep-de")
			kube := fake.NewClientset(pod, callerPod)
			var minted atomic.Int32
			kube.PrependReactor("create", "serviceaccounts", func(action kubetesting.Action) (bool, runtime.Object, error) {
				minted.Add(1)
				request := action.(kubetesting.CreateAction).GetObject().(*authenticationv1.TokenRequest).DeepCopy()
				require.Equal(t, []string{"ucp.radius-system.svc"}, request.Spec.Audiences)
				require.Equal(t, caller.PodUID, request.Spec.BoundObjectRef.UID)
				request.Status.Token = "observer"
				request.Status.ExpirationTimestamp = metav1.NewTime(time.Now().Add(time.Minute))
				return true, request, nil
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer observer", r.Header.Get("Authorization"))
				require.NoError(t, json.NewEncoder(w).Encode(mtls.InternalIdentityReceipt{
					SchemaVersion: 1, Provider: "external", Mode: mode, Component: "ucp", Namespace: target.Namespace,
					PodUID: string(target.PodUID), BootID: uuid.NewString(),
					Caller: "system:serviceaccount:radius-system:bicep-de", Challenge: r.URL.Query().Get("challenge"),
					ObservedAt: time.Now().UTC(),
				}))
			}))
			defer server.Close()
			observer := externalIdentityObserver{kube: kube, open: func(context.Context, externalIdentityTarget) (string, func(), error) {
				return server.URL, func() {}, nil
			}}
			evidence, err := observer.observe(t.Context(), mode, target, caller, nil)
			if mode == authz.ModeOff {
				require.Error(t, err)
				require.Zero(t, minted.Load())
			} else {
				require.NoError(t, err)
				require.Equal(t, string(target.PodUID), evidence.PodUID)
				require.Nil(t, evidence.Upstream)
				require.EqualValues(t, 1, minted.Load())
			}
		})
	}
}

func TestExternalIdentityPodPortForward(t *testing.T) {
	t.Parallel()
	target, _ := identityTargetFixture("ucp")
	for _, scenario := range []string{"success", "denied", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer receiver-specific", r.Header.Get("Authorization"))
				_, err := io.WriteString(w, "pod endpoint")
				require.NoError(t, err)
			}))
			defer receiver.Close()
			api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/namespaces/radius-system/pods/ucp-one/portforward", r.URL.Path)
				require.Equal(t, "Bearer observer-api", r.Header.Get("Authorization"))
				if scenario == "denied" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				_, err := httpstream.Handshake(r, w, []string{"portforward.k8s.io"})
				require.NoError(t, err)
				connection := streamspdy.NewResponseUpgrader().UpgradeResponse(w, r, func(stream httpstream.Stream, replied <-chan struct{}) error {
					if stream.Headers().Get(corev1.StreamType) != corev1.StreamTypeData {
						return nil
					}
					go func() {
						<-replied
						defer stream.Close()
						socket, err := net.DialTimeout("tcp", receiver.Listener.Addr().String(), 5*time.Second)
						if err != nil {
							t.Error(err)
							return
						}
						defer socket.Close()
						done := make(chan struct{})
						go func() {
							defer close(done)
							_, _ = io.Copy(socket, stream)
							socket.Close()
						}()
						_, _ = io.Copy(stream, socket)
						stream.Close()
						<-done
					}()
					return nil
				})
				if connection != nil {
					defer connection.Close()
					<-connection.CloseChan()
				}
			}))
			defer api.Close()
			observer := externalIdentityObserver{config: &rest.Config{Host: api.URL, BearerToken: "observer-api",
				TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: api.Certificate().Raw})}}}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			origin, stop, err := observer.portForward(ctx, target)
			if scenario != "success" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			defer stop()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin, nil)
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer receiver-specific")
			response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
			require.NoError(t, err)
			data, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, "pod endpoint", string(data))
			stop()
		})
	}
}
