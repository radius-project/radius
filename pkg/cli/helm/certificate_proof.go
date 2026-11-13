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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// TLSProofTarget identifies a specific expected workload pod and its public TLS material.
// Callers must discover its ownership/template and re-list all consumers before phase transitions.
type TLSProofTarget struct {
	Namespace         string
	PodName           string
	PodUID            types.UID
	ContainerName     string
	Port              uint16
	TrustBundleSHA256 string
	LeafSHA256        string
}

// PodTLSProof binds initialization evidence to Kubernetes-observed container identity.
// It is not a successful peer handshake, a process-drain barrier, or a rotation authorization.
type PodTLSProof struct {
	Evidence     mtls.StartupEvidence
	ContainerID  string
	RestartCount int32
}

// ObserveTLSStartupProof refuses observation until a lawful CLI observer identity
// is available. The legacy transport below is retained only for fixtures.
func ObserveTLSStartupProof(ctx context.Context, config *rest.Config, target TLSProofTarget) (PodTLSProof, error) {
	return PodTLSProof{}, errCertificateObserverUnavailable
}

func observeLegacyTLSStartupFixture(ctx context.Context, config *rest.Config, target TLSProofTarget) (PodTLSProof, error) {
	return observePodTLSProof(ctx, config, target, mtls.StartupProofPath, func(evidence mtls.StartupEvidence) error {
		return evidence.Validate(string(target.PodUID), target.TrustBundleSHA256, target.LeafSHA256, time.Now())
	})
}

func observePodTLSProof(ctx context.Context, config *rest.Config, target TLSProofTarget, path string, validate func(mtls.StartupEvidence) error) (PodTLSProof, error) {
	if config == nil || len(validation.IsDNS1123Label(target.Namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(target.PodName)) != 0 || target.PodUID == "" ||
		len(validation.IsDNS1123Label(target.ContainerName)) != 0 || target.Port == 0 {
		return PodTLSProof{}, errors.New("invalid TLS proof pod target")
	}
	config = rest.CopyConfig(config)
	config.Timeout = 10 * time.Second
	client, err := rest.HTTPClientFor(config)
	if err != nil {
		return PodTLSProof{}, fmt.Errorf("cannot prepare authenticated TLS proof observation: %w", err)
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	kube, err := kubernetes.NewForConfigAndClient(config, client)
	if err != nil {
		return PodTLSProof{}, fmt.Errorf("cannot prepare Kubernetes TLS proof client: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	pods := kube.CoreV1().Pods(target.Namespace)
	before, err := pods.Get(ctx, target.PodName, metav1.GetOptions{})
	if err != nil {
		return PodTLSProof{}, fmt.Errorf("cannot inspect TLS proof pod; check pod read permissions: %w", err)
	}
	container, err := proofContainer(before, target)
	if err != nil {
		return PodTLSProof{}, err
	}
	origin, stop, err := openPodProofPortForward(ctx, config, target)
	if err != nil {
		return PodTLSProof{}, err
	}
	defer stop()
	// Kubernetes authentication belongs only on the API connection, never on a
	// workload's public proof endpoint. Do not use environment proxies locally.
	proofTransport := &http.Transport{DisableKeepAlives: true, MaxResponseHeaderBytes: 8192}
	defer proofTransport.CloseIdleConnections()
	proofClient := &http.Client{Transport: proofTransport, Timeout: config.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+path, nil)
	if err != nil {
		return PodTLSProof{}, errors.New("cannot construct TLS startup proof request")
	}
	response, err := proofClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return PodTLSProof{}, fmt.Errorf("TLS startup proof observation interrupted: %w", ctx.Err())
		}
		return PodTLSProof{}, errors.New("cannot read TLS startup proof through authenticated pod port-forward")
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return PodTLSProof{}, errors.New("TLS startup proof endpoint redirects are not permitted")
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return PodTLSProof{}, errors.New("TLS startup proof endpoint access denied")
	}
	if response.StatusCode != http.StatusOK {
		return PodTLSProof{}, fmt.Errorf("TLS startup proof endpoint is unavailable (HTTP %d)", response.StatusCode)
	}
	const maxProofBytes = 8192
	data, err := io.ReadAll(io.LimitReader(response.Body, maxProofBytes+1))
	if err != nil || len(data) > maxProofBytes {
		if ctx.Err() != nil {
			return PodTLSProof{}, fmt.Errorf("TLS startup proof observation interrupted: %w", ctx.Err())
		}
		return PodTLSProof{}, errors.New("TLS startup proof is unreadable or exceeds its size limit")
	}
	var evidence mtls.StartupEvidence
	if err := decodeTLSProof(data, &evidence); err != nil {
		return PodTLSProof{}, err
	}
	if err := validate(evidence); err != nil {
		return PodTLSProof{}, err
	}
	if evidence.InitializedAt.Before(container.State.Running.StartedAt.Time) {
		return PodTLSProof{}, errors.New("TLS startup proof predates the running container")
	}
	after, err := pods.Get(ctx, target.PodName, metav1.GetOptions{})
	if err != nil {
		return PodTLSProof{}, fmt.Errorf("cannot recheck TLS proof pod identity: %w", err)
	}
	current, err := proofContainer(after, target)
	if err != nil {
		return PodTLSProof{}, err
	}
	if current.ContainerID != container.ContainerID || current.RestartCount != container.RestartCount ||
		!current.State.Running.StartedAt.Equal(&container.State.Running.StartedAt) {
		return PodTLSProof{}, errors.New("TLS proof pod container restarted during observation")
	}
	return PodTLSProof{Evidence: evidence, ContainerID: container.ContainerID, RestartCount: container.RestartCount}, nil
}

func proofContainer(pod *corev1.Pod, target TLSProofTarget) (corev1.ContainerStatus, error) {
	if pod.UID != target.PodUID || pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return corev1.ContainerStatus{}, errors.New("TLS proof pod was replaced, is terminating, or is not running")
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == target.ContainerName && status.State.Running != nil &&
			!status.State.Running.StartedAt.IsZero() && status.ContainerID != "" {
			return status, nil
		}
	}
	return corev1.ContainerStatus{}, errors.New("TLS proof container is not running")
}

func decodeTLSProof(data []byte, evidence *mtls.StartupEvidence) error {
	return decodeCertificateProof(data, evidence, []string{"schemaVersion", "ready", "proofKind", "podUid", "bootId", "initializedAt",
		"trustBundleSha256", "caSha256", "leafSha256", "notBefore", "notAfter", "peerHandshakeVerified"})
}

func decodeCertificateProof(data []byte, evidence any, required []string) error {
	if len(data) == 0 || len(data) > 8192 {
		return errors.New("TLS proof is empty or exceeds its size limit")
	}
	// A map detects duplicate top-level names before typed decoding. Duplicate names
	// could otherwise let different clients disagree about ready or identity.
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("invalid TLS startup proof response")
	}
	names := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return errors.New("invalid TLS startup proof response")
		}
		name, ok := token.(string)
		if !ok || names[name] {
			return errors.New("duplicate TLS startup proof field")
		}
		names[name] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return errors.New("invalid TLS startup proof response")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("null TLS startup proof field")
		}
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("invalid TLS startup proof response")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("unexpected trailing TLS startup proof data")
	}
	for _, name := range required {
		if !names[name] {
			return errors.New("missing TLS startup proof field")
		}
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(evidence); err != nil {
		return errors.New("invalid TLS startup proof schema")
	}
	return nil
}
