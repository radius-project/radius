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
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
)

func TestAggregationServingInventory(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"success", "overlap with new-only serving", "extra pod", "missing endpoint", "foreign endpoint", "wrong address", "wrong port", "rolling", "unready routing", "private source"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			ca := newTestCA(t, "public")
			meta := func(name string) metav1.ObjectMeta {
				return metav1.ObjectMeta{Name: name, Namespace: RadiusSystemNamespace, UID: types.UID(name), ResourceVersion: "1"}
			}
			source := &corev1.Secret{ObjectMeta: meta(ucpAPIServerTrustSecretName), Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{caCertificateKey: ca.pem}}
			source.Labels = map[string]string{helmManagedByLabel: helmManagedByValue}
			source.Annotations = map[string]string{helmReleaseNameAnnotation: radiusReleaseName, helmReleaseNamespaceAnnotation: RadiusSystemNamespace, allowDirectInjectionAnnotation: "true"}
			service := &corev1.Service{ObjectMeta: meta("ucp"), Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "ucp"}, Ports: []corev1.ServicePort{{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(9443)}}}}
			deployment := &appsv1.Deployment{ObjectMeta: meta("ucp"),
				Spec:   appsv1.DeploymentSpec{Replicas: ptr.To(int32(1)), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{ServiceAccountName: "ucp"}}},
				Status: appsv1.DeploymentStatus{ObservedGeneration: 7, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}}
			deployment.Generation = 7
			account := &corev1.ServiceAccount{ObjectMeta: meta("ucp")}
			pod := &corev1.Pod{ObjectMeta: meta("ucp-one"), Status: corev1.PodStatus{PodIPs: []corev1.PodIP{{IP: "10.0.0.1"}}}}
			pod.Labels = map[string]string{"app": "ucp"}
			endpoints := &discoveryv1.EndpointSlice{ObjectMeta: meta("ucp-endpoints"), AddressType: discoveryv1.AddressTypeIPv4,
				Ports: []discoveryv1.EndpointPort{{Name: ptr.To("https"), Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(int32(9443))}},
				Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.1"}, Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
					TargetRef: &corev1.ObjectReference{Kind: "Pod", Namespace: RadiusSystemNamespace, Name: pod.Name, UID: pod.UID}}}}
			endpoints.Labels = map[string]string{discoveryv1.LabelServiceName: service.Name}
			endpoints.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "Service", Name: service.Name, UID: service.UID, Controller: ptr.To(true)}}
			switch scenario {
			case "extra pod":
				deployment.Spec.Replicas = ptr.To(int32(2))
			case "missing endpoint":
				endpoints.Endpoints = nil
			case "foreign endpoint":
				endpoints.Endpoints[0].TargetRef.UID = "other"
			case "wrong address":
				endpoints.Endpoints[0].Addresses = []string{"10.0.0.99"}
			case "wrong port":
				endpoints.Ports[0].Port = ptr.To(int32(9444))
			case "rolling":
				deployment.Status.UpdatedReplicas = 0
			case "unready routing":
				service.Spec.PublishNotReadyAddresses = true
			case "private source":
				source.Data["tls.key"] = []byte("must-not-consume")
			}
			if scenario == "overlap with new-only serving" {
				source.Data[caCertificateKey] = append(append([]byte{}, newTestCA(t, "previous").pem...), ca.pem...)
			}
			kube := kubefake.NewClientset(source, service, deployment, account, pod, endpoints)
			inventory, err := readAggregationServingInventory(t.Context(), kube, RadiusSystemNamespace)
			if scenario == "overlap with new-only serving" {
				require.Error(t, err, "fresh-bootstrap inventory still requires one CA")
				inventory, err = readAggregationServingInventoryForCA(t.Context(), kube, RadiusSystemNamespace, source.Data[caCertificateKey], ca.pem)
			}
			if scenario != "success" && scenario != "overlap with new-only serving" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, inventory.targets, 1)
			require.Len(t, inventory.versions, 6)
			require.Equal(t, uint16(9443), inventory.targets[0].Port)
			require.Equal(t, deployment.Generation, inventory.targets[0].WorkloadGeneration)
			require.Equal(t, ca.pem, inventory.targets[0].CA)
			require.Equal(t, account.UID, inventory.targets[0].ServiceAccountUID)
			require.Equal(t, pod.UID, inventory.targets[0].PodUID)
		})
	}
}

func TestObserveServingCertificate(t *testing.T) {
	t.Parallel()
	for _, profile := range []string{"DNS", "SPIFFE"} {
		for _, scenario := range []string{"success", "wrong CA", "wrong SAN", "not ready", "SA replaced", "workload replaced", "restarted", "cancel TLS", "driver replaced", "not mounted"} {
			if profile == "DNS" && (scenario == "driver replaced" || scenario == "not mounted") {
				continue
			}
			t.Run(profile+"/"+scenario, func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				ca := newTestCA(t, "selected")
				target := ServingCertificateTarget{
					TLSProofTarget: TLSProofTarget{Namespace: RadiusSystemNamespace, PodName: "ucp-one",
						PodUID: "pod-one", ContainerName: "ucp", Port: 6443},
					WorkloadUID: "deployment-one", WorkloadGeneration: 7,
					ServiceAccount: "ucp", ServiceAccountUID: "sa-one",
					CA: ca.pem, DNSName: "ucp.radius-system.svc",
				}
				now := time.Now().UTC().Truncate(time.Second)
				leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(30 * time.Minute),
					KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
					DNSNames: []string{target.DNSName}}
				if profile == "SPIFFE" {
					target.DNSName, target.SPIFFEID = "", mtls.SPIFFEID(mtls.DefaultTrustDomain, target.Namespace, target.ServiceAccount)
					target.Driver, target.DriverUID = "csi.cert-manager.io", "driver-one"
					identity, err := url.Parse(target.SPIFFEID)
					require.NoError(t, err)
					leaf.URIs, leaf.DNSNames = []*url.URL{identity}, nil
				}
				if scenario == "wrong SAN" {
					leaf.DNSNames, leaf.URIs = []string{"other.radius-system.svc"}, nil
				}
				key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				require.NoError(t, err)
				der, err := x509.CreateCertificate(rand.Reader, leaf, ca.cert, &key.PublicKey, ca.key)
				require.NoError(t, err)
				var requests atomic.Int32
				receiver := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					requests.Add(1)
				}))
				receiver.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
					GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
						if scenario == "cancel TLS" {
							cancel()
						}
						return nil, nil
					}}
				receiver.StartTLS()
				defer receiver.Close()
				if scenario == "wrong CA" {
					target.CA = newTestCA(t, "other").pem
				}
				var reads atomic.Int32
				api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, "Bearer kube-api-only", r.Header.Get("Authorization"))
					w.Header().Set("Content-Type", "application/json")
					var object any
					switch {
					case strings.HasSuffix(r.URL.Path, "/pods/ucp-one/portforward"):
						servePodProofForward(t, w, r, receiver.Listener.Addr().String(), target.Port)
						return
					case strings.HasSuffix(r.URL.Path, "/pods/ucp-one"):
						pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: target.PodName, Namespace: target.Namespace, UID: target.PodUID,
							OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "ucp-rs", UID: "rs-one", Controller: ptr.To(true)}}},
							Spec: corev1.PodSpec{ServiceAccountName: target.ServiceAccount, Containers: []corev1.Container{{Name: target.ContainerName,
								Ports:        []corev1.ContainerPort{{ContainerPort: int32(target.Port), Protocol: corev1.ProtocolTCP}},
								VolumeMounts: []corev1.VolumeMount{{Name: "certificate", MountPath: "/var/run/radius/certs", ReadOnly: true}}}},
								Volumes: []corev1.Volume{{Name: "certificate", VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: target.Driver}}}}},
							Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
								ContainerStatuses: []corev1.ContainerStatus{{Name: target.ContainerName, Ready: true, ContainerID: "containerd://one",
									State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(now.Add(-time.Minute))}}}}},
						}
						if count := reads.Add(1); count > 1 && scenario == "restarted" {
							pod.Status.ContainerStatuses[0].ContainerID = "containerd://two"
						}
						if scenario == "not ready" {
							pod.Status.ContainerStatuses[0].Ready = false
						}
						if scenario == "not mounted" {
							pod.Spec.Containers[0].VolumeMounts = nil
						}
						object = pod
					case strings.HasSuffix(r.URL.Path, "/serviceaccounts/ucp"):
						uid := target.ServiceAccountUID
						if scenario == "SA replaced" {
							uid = "sa-two"
						}
						object = &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: target.ServiceAccount, UID: uid}}
					case strings.HasSuffix(r.URL.Path, "/replicasets/ucp-rs"):
						object = &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{UID: "rs-one",
							OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: "ucp", UID: target.WorkloadUID, Controller: ptr.To(true)}}}}
					case strings.HasSuffix(r.URL.Path, "/deployments/ucp"):
						generation := target.WorkloadGeneration
						if scenario == "workload replaced" {
							generation++
						}
						object = &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{UID: target.WorkloadUID, Generation: generation},
							Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{ServiceAccountName: target.ServiceAccount}}}}
					case strings.HasSuffix(r.URL.Path, "/csidrivers/"+target.Driver):
						uid := target.DriverUID
						if scenario == "driver replaced" {
							uid = types.UID("driver-two")
						}
						object = &storagev1.CSIDriver{ObjectMeta: metav1.ObjectMeta{UID: uid}}
					default:
						t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
						http.NotFound(w, r)
						return
					}
					require.Equal(t, http.MethodGet, r.Method)
					require.NoError(t, json.NewEncoder(w).Encode(object))
				}))
				defer api.Close()
				config := &rest.Config{Host: api.URL, BearerToken: "kube-api-only",
					TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: api.Certificate().Raw})}}
				result, err := ObserveServingCertificate(ctx, config, target)
				if scenario == "success" {
					require.NoError(t, err)
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(der)), result.LeafSHA256)
					require.Len(t, result.ChainSHA256, 2)
					require.Equal(t, leaf.NotAfter, result.NotAfter)
					require.Equal(t, target.PodUID, result.PodUID)
					require.EqualValues(t, 2, reads.Load())
				} else {
					require.Error(t, err)
					require.Zero(t, result)
					if scenario == "cancel TLS" {
						require.ErrorIs(t, err, context.Canceled)
					}
				}
				require.Zero(t, requests.Load(), "TLS measurement must send no workload HTTP request or token")
			})
		}
	}
}
