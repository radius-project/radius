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
	"errors"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

type podTrustEvidence struct {
	Evidence     mtls.TrustEvidence
	ContainerID  string
	RestartCount int32
}

// validateOutboundTrustEvidence consumes an already authenticated observation.
// It neither obtains credentials nor exposes a route. Production CLI acquisition
// remains unavailable; callers must separately bind the full consumer inventory.
func validateOutboundTrustEvidence(data []byte, target TLSProofTarget, bundle []byte, serverID string,
	before, after *corev1.Pod, now time.Time) (podTrustEvidence, error) {
	if before == nil || after == nil || target.LeafSHA256 != "" ||
		len(validation.IsDNS1123Label(target.Namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(target.PodName)) != 0 ||
		len(validation.IsDNS1123Label(target.ContainerName)) != 0 ||
		before.Namespace != target.Namespace || after.Namespace != target.Namespace ||
		before.Name != target.PodName || after.Name != target.PodName {
		return podTrustEvidence{}, errors.New("outbound trust evidence requires an exact pod and no client-leaf claim")
	}
	if err := exactCertificateSet(bundle, bundle, now); err != nil {
		return podTrustEvidence{}, err
	}
	digest, _, err := mtls.TrustBundleDigest(bundle, now)
	if err != nil || digest != target.TrustBundleSHA256 {
		return podTrustEvidence{}, errors.New("outbound trust target differs from the selected exact anchors")
	}
	var evidence mtls.TrustEvidence
	if err := decodeCertificateProof(data, &evidence, []string{
		"schemaVersion", "ready", "proofKind", "podUid", "bootId", "initializedAt",
		"trustBundleSha256", "caSha256", "notBefore", "notAfter", "expectedServerSpiffeId", "peerHandshakeVerified",
	}); err != nil {
		return podTrustEvidence{}, err
	}
	if err := evidence.Validate(string(target.PodUID), digest, serverID, now); err != nil {
		return podTrustEvidence{}, err
	}
	certificates, err := parseCertificateBundlePEM(bundle)
	if err != nil {
		return podTrustEvidence{}, err
	}
	notBefore, notAfter := certificates[0].NotBefore, certificates[0].NotAfter
	for _, certificate := range certificates[1:] {
		if certificate.NotBefore.After(notBefore) {
			notBefore = certificate.NotBefore
		}
		if certificate.NotAfter.Before(notAfter) {
			notAfter = certificate.NotAfter
		}
	}
	if !evidence.NotBefore.Equal(notBefore) || !evidence.NotAfter.Equal(notAfter) {
		return podTrustEvidence{}, errors.New("outbound trust evidence does not report the current selected-anchor validity intersection")
	}
	container, err := proofContainer(before, target)
	if err != nil {
		return podTrustEvidence{}, err
	}
	current, err := proofContainer(after, target)
	if err != nil {
		return podTrustEvidence{}, err
	}
	if container.ContainerID != current.ContainerID || container.RestartCount != current.RestartCount ||
		!container.State.Running.StartedAt.Equal(&current.State.Running.StartedAt) ||
		evidence.InitializedAt.Before(container.State.Running.StartedAt.Time) {
		return podTrustEvidence{}, errors.New("outbound trust process identity changed during observation or predates the running container")
	}
	return podTrustEvidence{Evidence: evidence, ContainerID: container.ContainerID, RestartCount: container.RestartCount}, nil
}
