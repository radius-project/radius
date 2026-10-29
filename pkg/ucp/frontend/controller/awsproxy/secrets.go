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

package awsproxy

import (
	"encoding/json"
	"strings"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
)

// unmarkedSecrets are properties that a caller sends with a secret value although the type's CloudFormation schema
// does not mark them write-only. They were found by scanning the CloudFormation registry (us-west-2) for string
// properties a caller can set whose name says they hold a secret. Paths are relative to the properties object; "*"
// stands for every array item.
var unmarkedSecrets = map[string][]string{
	"AWS::EC2::VerifiedAccessTrustProvider":       {"OidcOptions/ClientSecret"},
	"AWS::Invoicing::ProcurementPortalPreference": {"TestEnvPreference/ProcurementPortalSharedSecret"},
}

// secretProperties returns the paths of the properties whose request values must never be echoed in a response: the
// write-only properties of the type schema, plus the secrets the schema does not mark write-only.
func secretProperties(typeName string, schema string) ([]string, error) {
	var parsed awsoperations.ResourceTypeSchema
	if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
		return nil, err
	}

	paths := []string{}
	for _, p := range parsed.WriteOnlyProperties {
		if name, err := awsoperations.ParsePropertyName(p); err == nil {
			paths = append(paths, name)
		}
	}

	for k, v := range unmarkedSecrets {
		if strings.EqualFold(k, typeName) {
			paths = append(paths, v...)
		}
	}
	return paths, nil
}

// buildResponseProperties returns the properties of a create or update response: the current properties AWS returned,
// overlaid with the request properties. Request values at the secret paths are left out, so a response never echoes a
// secret the caller sent. Values AWS returned are passed through unchanged, including inside objects.
func buildResponseProperties(current map[string]any, request map[string]any, secrets []string) map[string]any {
	out, _ := deepCopy(current).(map[string]any)
	if out == nil {
		out = map[string]any{}
	}

	echoed, _ := deepCopy(request).(map[string]any)
	removePaths(echoed, secrets)
	mergeProperties(out, echoed)
	return out
}

// mergeProperties copies src into dst. When both hold an object under the same key the objects are merged, so values
// in dst inside an object stay unless src sets them.
func mergeProperties(dst map[string]any, src map[string]any) {
	for k, v := range src {
		srcMap, srcIsMap := v.(map[string]any)
		dstMap, dstIsMap := dst[k].(map[string]any)
		if srcIsMap && dstIsMap {
			mergeProperties(dstMap, srcMap)
			continue
		}
		dst[k] = v
	}
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = deepCopy(item)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = deepCopy(item)
		}
		return out
	default:
		return v
	}
}

// removePaths removes the properties at paths from properties in place. A "*" segment matches every array item.
func removePaths(properties map[string]any, paths []string) {
	for _, p := range paths {
		removePath(properties, strings.Split(p, "/"))
	}
}

func removePath(obj map[string]any, segments []string) {
	if obj == nil || len(segments) == 0 {
		return
	}
	if len(segments) == 1 {
		delete(obj, segments[0])
		return
	}

	value := obj[segments[0]]
	if segments[1] == "*" {
		if len(segments) == 2 {
			delete(obj, segments[0])
			return
		}
		items, _ := value.([]any)
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				removePath(m, segments[2:])
			}
		}
		return
	}

	if m, ok := value.(map[string]any); ok {
		removePath(m, segments[1:])
	}
}
