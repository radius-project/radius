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
	"encoding/hex"
	"encoding/json"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const (
	aggregationBootstrapName  = "radius-aggregation-bootstrap"
	bootstrapNamespaceIntent  = "radius.io/aggregation-bootstrap-intent"
	aggregationBootstrapOwner = "radius.io/aggregation-bootstrap"
)

type aggregationBootstrapPin struct {
	UID             types.UID `json:"uid"`
	ResourceVersion string    `json:"resourceVersion"`
	SpecSHA256      string    `json:"specSHA256"`
}

func bootstrapResourcePin(object *unstructured.Unstructured) (aggregationBootstrapPin, error) {
	if object.GetUID() == "" || object.GetResourceVersion() == "" || object.GetDeletionTimestamp() != nil {
		return aggregationBootstrapPin{}, errors.New("bootstrap resource has no stable live identity")
	}
	spec, found, err := unstructured.NestedMap(object.Object, "spec")
	if err != nil || !found {
		return aggregationBootstrapPin{}, errors.New("bootstrap resource has no valid specification")
	}
	digest, err := bootstrapDigest(spec)
	if err != nil {
		return aggregationBootstrapPin{}, err
	}
	return aggregationBootstrapPin{UID: object.GetUID(), ResourceVersion: object.GetResourceVersion(), SpecSHA256: digest}, nil
}

func rotationIssuerSpecDigest(issuer *unstructured.Unstructured) (string, error) {
	spec, found, err := unstructured.NestedMap(issuer.Object, "spec")
	if err != nil || !found || len(spec) == 0 {
		return "", errors.New("external issuer has no valid specification")
	}
	return bootstrapDigest(spec)
}

func bootstrapDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Any legacy journal or creation intent requires the matching pre-token release.
// Its schema cannot authorize token/CSI operations, so do not decode or resume it.
func (s *rotationStore) hasLegacyBootstrap(ctx context.Context) (bool, error) {
	_, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, aggregationBootstrapName, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		return err == nil, err
	}
	namespace, err := s.kube.CoreV1().Namespaces().Get(ctx, s.namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, found := namespace.Annotations[bootstrapNamespaceIntent]
	return found, nil
}
