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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// ServingCertificateTarget pins a receiver pod and selected public server trust.
// Aggregation uses DNS; CSI receivers use an exact SPIFFE ID and observed driver.
type ServingCertificateTarget struct {
	TLSProofTarget
	WorkloadUID        types.UID
	WorkloadGeneration int64
	ServiceAccount     string
	ServiceAccountUID  types.UID
	CA                 []byte
	DNSName            string
	SPIFFEID           string
	Driver             string
	DriverUID          types.UID
}

// ServingCertificateObservation measures a real TLS handshake, not a component's
// outbound loaded trust or its caller-token readiness.
type ServingCertificateObservation struct {
	PodUID       types.UID
	ContainerID  string
	RestartCount int32
	LeafSHA256   string
	ChainSHA256  []string
	NotBefore    time.Time
	NotAfter     time.Time
}

// ObserveServingCertificate sends no workload HTTP request or bearer token. API
// credentials authenticate only the outer exact-pod tunnel and inventory reads.
func ObserveServingCertificate(ctx context.Context, config *rest.Config, target ServingCertificateTarget) (ServingCertificateObservation, error) {
	var result ServingCertificateObservation
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if config == nil || target.PodUID == "" || target.WorkloadUID == "" || target.WorkloadGeneration <= 0 ||
		target.ServiceAccount == "" || target.ServiceAccountUID == "" ||
		target.Port == 0 || (target.DNSName == "") == (target.SPIFFEID == "") {
		return result, errors.New("serving observation requires an exact pod, workload and one server identity")
	}
	if target.DNSName != "" && (target.DNSName != "ucp."+target.Namespace+".svc" ||
		target.Driver != "" || target.DriverUID != "") {
		return result, errors.New("aggregation requires the canonical UCP DNS identity without CSI evidence")
	}
	if target.SPIFFEID != "" && (target.Driver == "" || target.DriverUID == "") {
		return result, errors.New("CSI receiver observation requires an observed driver")
	}
	if target.SPIFFEID != "" {
		id, err := url.Parse(target.SPIFFEID)
		if err != nil || id.Host == "" || target.SPIFFEID != mtls.SPIFFEID(id.Host, target.Namespace, target.ServiceAccount) {
			return result, errors.New("CSI server identity does not match the pinned namespace and ServiceAccount")
		}
	}
	if _, _, err := mtls.TrustBundleDigest(target.CA, time.Now()); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	config = rest.CopyConfig(config)
	config.Timeout = 10 * time.Second
	client, err := rest.HTTPClientFor(config)
	if err != nil {
		return result, err
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	kube, err := kubernetes.NewForConfigAndClient(config, client)
	if err != nil {
		return result, err
	}
	before, err := observedServingPod(ctx, kube, target)
	if err != nil {
		return result, err
	}
	container, err := proofContainer(before, target.TLSProofTarget)
	if err != nil {
		return result, err
	}
	origin, stop, err := openPodProofPortForward(ctx, config, target.TLSProofTarget)
	if err != nil {
		return result, err
	}
	defer stop()
	address, err := url.Parse(origin)
	if err != nil {
		return result, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(target.CA)
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool, ServerName: target.DNSName}
	var chain []*x509.Certificate
	tlsConfig.VerifyConnection = func(connection tls.ConnectionState) error {
		if target.SPIFFEID != "" {
			verified, err := mtls.VerifyServerChain(target.CA, connection.PeerCertificates, target.SPIFFEID, time.Now())
			chain = verified
			return err
		}
		if len(connection.VerifiedChains) == 0 {
			return errors.New("aggregation server chain is unverified")
		}
		leaf := connection.PeerCertificates[0]
		if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != target.DNSName ||
			len(leaf.URIs) != 0 || len(leaf.IPAddresses) != 0 || len(leaf.EmailAddresses) != 0 {
			return errors.New("aggregation serving certificate has an unexpected identity")
		}
		chain = connection.VerifiedChains[0]
		return nil
	}
	if target.SPIFFEID != "" {
		// URI identity and full chain are verified by VerifyServerChain above.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	}
	dialer := &tls.Dialer{NetDialer: &net.Dialer{}, Config: tlsConfig}
	conn, err := dialer.DialContext(ctx, "tcp", address.Host)
	if err != nil {
		return result, fmt.Errorf("exact-pod serving handshake failed: %w", err)
	}
	if err := conn.Close(); err != nil {
		return result, err
	}
	after, err := observedServingPod(ctx, kube, target)
	if err != nil {
		return result, err
	}
	current, err := proofContainer(after, target.TLSProofTarget)
	if err != nil || container.ContainerID != current.ContainerID || container.RestartCount != current.RestartCount ||
		!container.State.Running.StartedAt.Equal(&current.State.Running.StartedAt) {
		return result, errors.New("serving pod changed during the handshake")
	}
	result.PodUID, result.ContainerID, result.RestartCount = target.PodUID, current.ContainerID, current.RestartCount
	for index, cert := range chain {
		hash := fmt.Sprintf("%x", sha256.Sum256(cert.Raw))
		result.ChainSHA256 = append(result.ChainSHA256, hash)
		if index == 0 {
			result.LeafSHA256, result.NotBefore, result.NotAfter = hash, cert.NotBefore, cert.NotAfter
		}
		if cert.NotBefore.After(result.NotBefore) {
			result.NotBefore = cert.NotBefore
		}
		if cert.NotAfter.Before(result.NotAfter) {
			result.NotAfter = cert.NotAfter
		}
	}
	return result, nil
}

func observedServingPod(ctx context.Context, kube kubernetes.Interface, target ServingCertificateTarget) (*corev1.Pod, error) {
	pod, err := kube.CoreV1().Pods(target.Namespace).Get(ctx, target.PodName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if pod.Spec.ServiceAccountName != target.ServiceAccount {
		return nil, errors.New("serving pod ServiceAccount changed")
	}
	if pod.DeletionTimestamp != nil || !slices.ContainsFunc(pod.Status.Conditions, func(condition corev1.PodCondition) bool {
		return condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue
	}) || !slices.ContainsFunc(pod.Status.ContainerStatuses, func(container corev1.ContainerStatus) bool {
		return container.Name == target.ContainerName && container.Ready
	}) {
		return nil, errors.New("serving pod is not Ready")
	}
	if !slices.ContainsFunc(pod.Spec.Containers, func(container corev1.Container) bool {
		return container.Name == target.ContainerName && slices.ContainsFunc(container.Ports, func(port corev1.ContainerPort) bool {
			return port.ContainerPort == int32(target.Port) && (port.Protocol == "" || port.Protocol == corev1.ProtocolTCP)
		})
	}) {
		return nil, errors.New("serving port is not declared by the pinned container")
	}
	account, err := kube.CoreV1().ServiceAccounts(target.Namespace).Get(ctx, target.ServiceAccount, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if account.UID != target.ServiceAccountUID || account.DeletionTimestamp != nil {
		return nil, errors.New("serving ServiceAccount identity changed")
	}
	owner := metav1.GetControllerOf(pod)
	if owner == nil || owner.Kind != "ReplicaSet" || owner.APIVersion != "apps/v1" {
		return nil, errors.New("serving pod has no pinned ReplicaSet")
	}
	replicaSet, err := kube.AppsV1().ReplicaSets(target.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	workload := metav1.GetControllerOf(replicaSet)
	if replicaSet.UID != owner.UID || replicaSet.DeletionTimestamp != nil || workload == nil ||
		workload.Kind != "Deployment" || workload.APIVersion != "apps/v1" || workload.UID != target.WorkloadUID {
		return nil, errors.New("serving pod workload ownership changed")
	}
	deployment, err := kube.AppsV1().Deployments(target.Namespace).Get(ctx, workload.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if deployment.UID != target.WorkloadUID || deployment.Generation != target.WorkloadGeneration || deployment.DeletionTimestamp != nil ||
		deployment.Spec.Template.Spec.ServiceAccountName != target.ServiceAccount {
		return nil, errors.New("serving deployment identity changed")
	}
	if target.SPIFFEID != "" {
		driver, err := kube.StorageV1().CSIDrivers().Get(ctx, target.Driver, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		mounted := false
		for _, container := range pod.Spec.Containers {
			if container.Name != target.ContainerName {
				continue
			}
			for _, mount := range container.VolumeMounts {
				mounted = mounted || slices.ContainsFunc(pod.Spec.Volumes, func(volume corev1.Volume) bool {
					return volume.Name == mount.Name && volume.CSI != nil && volume.CSI.Driver == target.Driver
				})
			}
		}
		if driver.UID != target.DriverUID || driver.DeletionTimestamp != nil || !mounted {
			return nil, errors.New("serving pod CSI driver changed or is not mounted")
		}
	}
	return pod, nil
}
