/*
Copyright 2023 The Radius Authors.

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

package kubeutil

import (
	"bytes"
	"io"

	"k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
)

// ObjectManifest is a map of runtime.Object slice where the key is GroupVersionKind for the resource.
type ObjectManifest map[schema.GroupVersionKind][]runtime.Object

// Get returns a runtime.Object slice for the given key.
func (m ObjectManifest) Get(gvk schema.GroupVersionKind) []runtime.Object {
	obj, ok := m[gvk]
	if ok {
		return obj
	} else {
		return []runtime.Object{}
	}
}

// GetFirst returns the first runtime.Object for the given key.
func (m ObjectManifest) GetFirst(gvk schema.GroupVersionKind) runtime.Object {
	obj, ok := m[gvk]
	if ok {
		return obj[0]
	} else {
		return nil
	}
}

// ParseManifest parses the given manifest and returns a map of runtime.Object slice where
// the key is GroupVersionKind for the resource.
// It returns an error if the given manifest is invalid.
func ParseManifest(data []byte) (ObjectManifest, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	deser := clientscheme.Codecs.UniversalDeserializer()

	objects := ObjectManifest{}
	for {
		ext := runtime.RawExtension{}
		if err := decoder.Decode(&ext); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		obj, _, err := deser.Decode([]byte(ext.Raw), nil, nil)
		if err != nil {
			return nil, err
		}

		key := obj.GetObjectKind().GroupVersionKind()
		if v, ok := objects[key]; ok {
			objects[key] = append(v, obj)
		} else {
			objects[key] = []runtime.Object{obj}
		}
	}

	return objects, nil
}
