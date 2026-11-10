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
	"io"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

// Before a retry, compare surviving Helm resources with the original desired
// specification. A matching marker alone must not authorize overwriting drift.
func (s *rotationStore) checkBootstrapInstall(ctx context.Context, id, manifest string, requireAll bool) error {
	_, state, err := s.readBootstrap(ctx)
	if err != nil {
		return err
	}
	if state.ID != id || state.Phase != bootstrapInstalling {
		return errors.New("bootstrap install no longer matches its recorded phase")
	}
	expected := map[string]schema.GroupVersionResource{
		"APIService/" + radiusAPIService: apiServiceResource,
		"Service/ucp":                    corev1.SchemeGroupVersion.WithResource("services"),
	}
	for _, name := range rotationComponents {
		if name == "ucp" || state.Scope == rotationComponentsAndAggregation {
			expected["Deployment/"+name] = appsv1.SchemeGroupVersion.WithResource("deployments")
		}
		if state.Scope == rotationComponentsAndAggregation {
			expected["Certificate/"+name+"-cert"] = rotationCertificateResource
		}
	}
	seen := map[string]bool{}
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		desired := &unstructured.Unstructured{}
		if err := decoder.Decode(desired); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		key := desired.GetKind() + "/" + desired.GetName()
		resource, required := expected[key]
		if !required {
			continue
		}
		namespace := s.namespace
		if resource == apiServiceResource {
			namespace = ""
		}
		if seen[key] || desired.GetAPIVersion() != resource.GroupVersion().String() ||
			desired.GetNamespace() != "" && desired.GetNamespace() != namespace ||
			desired.GetAnnotations()[aggregationBootstrapOwner] != id {
			return errors.New("selected bootstrap chart has inconsistent resource identity or creation intent")
		}
		seen[key] = true
		object, err := s.dynamic.Resource(resource).Namespace(namespace).Get(ctx, desired.GetName(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, pinned := state.Resources[bootstrapResourceKey(resource, desired.GetName())]
			if pinned || requireAll {
				return fmt.Errorf("bootstrap resource %s is missing; a pinned resource cannot be recreated", key)
			}
			continue
		}
		if err != nil {
			return err
		}
		if !bootstrapDeclaredSpec(object.Object["spec"], desired.Object["spec"]) {
			return fmt.Errorf("bootstrap resource %s changed its declared specification; it will not be overwritten", key)
		}
		if err := s.recordBootstrapResource(ctx, id, resource, object); err != nil {
			return err
		}
	}
	if len(seen) != len(expected) {
		return errors.New("selected bootstrap chart is missing required installation resources")
	}
	return nil
}

func bootstrapDeclaredSpec(actual, desired any) bool {
	switch desired := desired.(type) {
	case map[string]any:
		current, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for key, value := range desired {
			if !bootstrapDeclaredSpec(current[key], value) {
				return false
			}
		}
		return true
	case []any:
		current, ok := actual.([]any)
		if !ok || len(current) != len(desired) {
			return false
		}
		for index, value := range desired {
			if !bootstrapDeclaredSpec(current[index], value) {
				return false
			}
		}
		return true
	default:
		return rotationJSONEqual(actual, desired)
	}
}
