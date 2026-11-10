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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func bootstrapResourceKey(resource schema.GroupVersionResource, name string) string {
	return resource.Group + "/" + resource.Resource + "/" + name
}

func (s *rotationStore) checkBootstrapResources(ctx context.Context, state aggregationBootstrapState) error {
	for key, pin := range state.Resources {
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			return errors.New("invalid bootstrap resource identity")
		}
		resource := schema.GroupVersionResource{Group: parts[0], Version: "v1", Resource: parts[1]}
		switch resource {
		case rotationIssuerResource, rotationCertificateResource,
			schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
			schema.GroupVersionResource{Version: "v1", Resource: "services"}, apiServiceResource:
		default:
			return errors.New("unsupported bootstrap resource identity")
		}
		namespace := s.namespace
		if resource == apiServiceResource {
			namespace = ""
		}
		object, err := s.dynamic.Resource(resource).Namespace(namespace).Get(ctx, parts[2], metav1.GetOptions{})
		if err != nil {
			return err
		}
		current, err := bootstrapResourcePin(object)
		if err != nil {
			return err
		}
		if current.UID != pin.UID || current.SpecSHA256 != pin.SpecSHA256 ||
			object.GetAnnotations()[aggregationBootstrapOwner] != state.ID || len(object.GetOwnerReferences()) != 0 {
			return errors.New("pinned bootstrap resource changed during installation")
		}
		issuanceOwned := resource == rotationIssuerResource && parts[2] != "radius-selfsigned-bootstrap" ||
			resource == rotationCertificateResource && strings.HasPrefix(parts[2], "ucp-cert-aggregation-")
		if issuanceOwned {
			if object.GetLabels()[rotationOwner] != s.release || object.GetLabels()[rotationTransaction] != state.ID ||
				object.GetAnnotations()["meta.helm.sh/release-name"] != "" {
				return errors.New("bootstrap issuance resource changed ownership")
			}
		} else if !rotationOwned(object, s.namespace, s.release) {
			return errors.New("bootstrap installation resource changed Helm ownership")
		}
	}
	return nil
}

func (s *rotationStore) recordBootstrapResource(ctx context.Context, transaction string, resource schema.GroupVersionResource, object *unstructured.Unstructured) error {
	journal, state, err := s.readBootstrap(ctx)
	if err != nil {
		return err
	}
	if state.ID != transaction || state.Phase != bootstrapVerifying && state.Phase != bootstrapInstalling {
		return errors.New("bootstrap resource binding requires its verifying intent")
	}
	pin, err := bootstrapResourcePin(object)
	if err != nil {
		return err
	}
	if object.GetAnnotations()[aggregationBootstrapOwner] != state.ID ||
		len(object.GetOwnerReferences()) != 0 || !rotationOwned(object, s.namespace, s.release) {
		return errors.New("bootstrap cannot bind a resource without matching creation intent and Helm ownership")
	}
	key := bootstrapResourceKey(resource, object.GetName())
	if previous, found := state.Resources[key]; found {
		if previous.UID != pin.UID || previous.SpecSHA256 != pin.SpecSHA256 {
			return errors.New("a bound bootstrap resource was replaced or changed")
		}
		return nil
	}
	next := state
	next.Resources = make(map[string]aggregationBootstrapPin, len(state.Resources)+1)
	for key, value := range state.Resources {
		next.Resources[key] = value
	}
	next.Resources[key] = pin
	_, err = s.updateBootstrap(ctx, journal, state, next)
	return err
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

// ensureBootstrapResource records an observed UID after a create-only write.
// The intent marker admits a retry after creation succeeded but recording failed;
// an unrelated pre-existing object is never adopted.
func (s *rotationStore) ensureBootstrapResource(ctx context.Context, transaction string, resource schema.GroupVersionResource, desired *unstructured.Unstructured, helmOwned bool) error {
	if resource != rotationIssuerResource && resource != rotationCertificateResource ||
		desired.GetName() == "" || desired.GetNamespace() != s.namespace {
		return errors.New("bootstrap creation is limited to namespace-local issuance resources")
	}
	client := s.dynamic.Resource(resource).Namespace(s.namespace)
	object, readErr := client.Get(ctx, desired.GetName(), metav1.GetOptions{})
	if readErr != nil && !apierrors.IsNotFound(readErr) {
		return readErr
	}
	journal, state, err := s.readBootstrap(ctx)
	if err != nil {
		return err
	}
	if state.ID != transaction || state.Phase != bootstrapPreparing {
		return errors.New("bootstrap creation intent changed")
	}
	key := bootstrapResourceKey(resource, desired.GetName())
	pinned, recorded := state.Resources[key]
	expected := desired.DeepCopy()
	labels := expected.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[rotationOwner] = s.release
	annotations := expected.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[aggregationBootstrapOwner] = state.ID
	if helmOwned {
		labels["app.kubernetes.io/managed-by"] = "Helm"
		annotations["meta.helm.sh/release-name"], annotations["meta.helm.sh/release-namespace"] = s.release, s.namespace
	}
	expected.SetLabels(labels)
	expected.SetAnnotations(annotations)
	if apierrors.IsNotFound(readErr) {
		if recorded {
			return errors.New("a pinned bootstrap resource was deleted; it will not be recreated")
		}
		if err := s.checkWrite(ctx); err != nil {
			return err
		}
		object, err = client.Create(ctx, expected, metav1.CreateOptions{FieldManager: FieldManager})
		if err != nil {
			return fmt.Errorf("cannot create bootstrap resource; reread intent before retrying: %w", err)
		}
	}
	expectedSpec, _, err := unstructured.NestedMap(expected.Object, "spec")
	if err != nil {
		return err
	}
	actualSpec, _, err := unstructured.NestedMap(object.Object, "spec")
	if err != nil {
		return err
	}
	if object.GetAnnotations()[aggregationBootstrapOwner] != state.ID ||
		object.GetLabels()[rotationOwner] != s.release || len(object.GetOwnerReferences()) != 0 ||
		helmOwned && !rotationOwned(object, s.namespace, s.release) ||
		!helmOwned && object.GetAnnotations()["meta.helm.sh/release-name"] != "" {
		return errors.New("bootstrap resource has foreign ownership or changed configuration")
	}
	for key, expected := range expectedSpec {
		if !rotationJSONEqual(actualSpec[key], expected) {
			return errors.New("bootstrap resource changed a declared specification field")
		}
	}
	pin, err := bootstrapResourcePin(object)
	if err != nil {
		return err
	}
	if recorded {
		if pin.UID != pinned.UID || pin.SpecSHA256 != pinned.SpecSHA256 {
			return errors.New("bootstrap resource was replaced or changed")
		}
		return nil
	}
	next := state
	next.Resources = make(map[string]aggregationBootstrapPin, len(state.Resources)+1)
	for key, value := range state.Resources {
		next.Resources[key] = value
	}
	next.Resources[key] = pin
	_, err = s.updateBootstrap(ctx, journal, state, next)
	return err
}
