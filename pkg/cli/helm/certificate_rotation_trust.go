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
	"errors"
	"fmt"

	"github.com/radius-project/radius/pkg/authz/mtls"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// publishComponentTrust creates public trust without changing any consumer.
// Callers supply only the explicitly selected dedicated component CA(s), never
// the legacy aggregation root or an issuer's organization chain.
func (s *rotationStore) publishComponentTrust(ctx context.Context, bundle []byte) (*corev1.ConfigMap, string, error) {
	if len(bundle) > maxRotationBytes {
		return nil, "", errors.New("component trust exceeds the public bundle size limit")
	}
	digest, roots, err := mtls.TrustBundleDigest(bundle, s.now())
	if err != nil || len(roots) > 2 {
		return nil, "", errors.New("component rotation trust must contain only the selected currently valid old and new dedicated CAs")
	}
	name := "radius-mtls-ca-" + digest[:32]
	maps := s.kube.CoreV1().ConfigMaps(s.namespace)
	current, err := maps.Get(ctx, name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, "", fmt.Errorf("cannot inspect component trust generation: %w", err)
	}
	if apierrors.IsNotFound(err) {
		current, err = maps.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.namespace, Labels: map[string]string{rotationOwner: s.release}},
			Immutable:  ptr.To(true),
			Data:       map[string]string{"ca.crt": string(bundle)},
		}, metav1.CreateOptions{FieldManager: FieldManager})
		if apierrors.IsAlreadyExists(err) {
			// A concurrent publisher may create the same immutable generation.
			// Read and validate it; never take over or overwrite its contents.
			current, err = maps.Get(ctx, name, metav1.GetOptions{})
		}
		if err != nil {
			return nil, "", fmt.Errorf("cannot publish component trust generation; inspect it before retrying: %w", err)
		}
	}
	actual, _, err := mtls.TrustBundleDigest([]byte(current.Data["ca.crt"]), s.now())
	if err != nil || actual != digest || current.UID == "" || current.DeletionTimestamp != nil ||
		current.Immutable == nil || !*current.Immutable || current.Labels[rotationOwner] != s.release ||
		len(current.OwnerReferences) != 0 || current.Annotations["meta.helm.sh/release-name"] != "" ||
		len(current.Data) != 1 || len(current.BinaryData) != 0 {
		return nil, "", errors.New("component trust generation has unexpected contents, mutability, or ownership; it was not overwritten")
	}
	return current, digest, nil
}
