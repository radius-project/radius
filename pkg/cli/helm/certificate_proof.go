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
	"encoding/json"
	"errors"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
