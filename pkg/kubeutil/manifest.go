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
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	scheme "k8s.io/client-go/kubernetes/scheme"
)

// ParseManifest parses the given manifest and returns a list of objects. It returns an error if the manifest is invalid.
func ParseManifest(data []byte) (map[string][]runtime.Object, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	deser := scheme.Codecs.UniversalDeserializer()

	objects := map[string][]runtime.Object{}
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
		resourceTypeName := strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind)
		if v, ok := objects[resourceTypeName]; ok {
			objects[resourceTypeName] = append(v, obj)
		} else {
			objects[resourceTypeName] = []runtime.Object{obj}
		}
	}

	return objects, nil
}
