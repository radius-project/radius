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
	"crypto/hmac"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/wI2L/jsondiff"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
)

// AWS never returns write-only values, such as passwords, so UCP cannot compare them with AWS. Instead, the record
// keeps an HMAC-SHA-256 fingerprint of each write-only value of the last successful deploy, made with a key that lives
// in a Kubernetes Secret and never in the data store. A redeploy compares the template's values with them.

// FingerprintKeyFunc returns the key for fingerprints of write-only values, or an error naming the Kubernetes Secret
// when the key is not available. aws.Module.FingerprintKey is one.
type FingerprintKeyFunc func() (*ucp_aws.FingerprintKey, error)

// fingerprintStatus is what the record's fingerprint says about a write-only value.
type fingerprintStatus int

const (
	// fingerprintUnknown means the record has no usable fingerprint for the value: UCP just took the resource over,
	// the record is older than fingerprints, or a different key made them. It is the zero value, so a path with no
	// status is unknown.
	fingerprintUnknown fingerprintStatus = iota

	// fingerprintUnchanged means the value matches the fingerprint of the last successful deploy.
	fingerprintUnchanged

	// fingerprintChanged means the value differs from the last successful deploy, or the last deploy did not set it.
	fingerprintChanged
)

// fingerprints makes the fingerprints of the write-only values in properties with the current key. It returns nil
// when properties set no write-only value, so those requests work without the key. When properties set a write-only
// value and the key is not available, it returns a 500 naming the Secret. put calls it before it saves anything or
// calls Cloud Control.
func (e *engine) fingerprints(r *request, schema *typeSchema, properties map[string]any) (*datamodel.AWSFingerprints, armrpc_rest.Response, error) {
	writeOnly := awsoperations.PropertyPaths(schema.WriteOnlyProperties)
	paths := fingerprintPaths(writeOnly)
	values := map[string]any{}
	for _, w := range writeOnly {
		// Only a write-only value the template sets needs a fingerprint, not the array or object that would hold it.
		if !hasValueAt(properties, strings.Split(w, "/")) {
			continue
		}
		path := coveringPath(paths, fingerprintPath(w))
		values[path], _ = valueAt(properties, path)
	}
	if len(values) == 0 {
		return nil, nil, nil
	}

	key, err := e.fingerprintKey()
	if err == nil && key == nil {
		err = errors.New("the AWS fingerprint key is not loaded")
	}
	if err != nil {
		return nil, fingerprintKeyUnavailable(r, slices.Sorted(maps.Keys(values)), err), nil
	}

	out := &datamodel.AWSFingerprints{KeyID: key.ID(), Digests: map[string]string{}}
	for path, value := range values {
		digest, err := key.Fingerprint(r.id.String(), path, value)
		if err != nil {
			return nil, nil, err
		}
		out.Digests[path] = digest
	}
	return out, nil, nil
}

func fingerprintKeyUnavailable(r *request, paths []string, err error) armrpc_rest.Response {
	return armrpc_rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
		Error: &v1.ErrorDetails{
			Code: v1.CodeInternal,
			Message: fmt.Sprintf("UCP cannot deploy the AWS resource %q, because it needs the key in the Kubernetes Secret %s to fingerprint these write-only properties: %s. Make sure the Secret exists and has a key, then restart UCP: %v",
				r.id.Name(), ucp_aws.FingerprintKeySecretName, strings.Join(paths, ", "), err),
		},
	})
}

// fingerprintPaths returns the paths the fingerprints of a type cover, sorted, for its write-only properties relative
// to the properties object. A write-only property inside array items, such as Users/*/Secret, is covered by a
// fingerprint of the whole array, Users, because an array item has no stable path. A path inside another one is
// covered by the outer fingerprint.
func fingerprintPaths(writeOnly []string) []string {
	paths := []string{}
	for _, w := range writeOnly {
		paths = append(paths, fingerprintPath(w))
	}
	slices.Sort(paths)

	out := []string{}
	for _, p := range paths {
		// Sorting puts a path before the paths inside it.
		if !slices.ContainsFunc(out, func(o string) bool { return p == o || strings.HasPrefix(p, o+"/") }) {
			out = append(out, p)
		}
	}
	return out
}

// fingerprintPath returns the path the fingerprint of a write-only property covers: the part before its first "*"
// segment.
func fingerprintPath(writeOnly string) string {
	segments := strings.Split(writeOnly, "/")
	if i := slices.Index(segments, "*"); i >= 0 {
		segments = segments[:i]
	}
	return strings.Join(segments, "/")
}

// coveringPath returns the path in paths, the result of fingerprintPaths, whose fingerprint covers the fingerprint
// path p: p itself, or a path p is inside.
func coveringPath(paths []string, p string) string {
	for _, c := range paths {
		if p == c || strings.HasPrefix(p, c+"/") {
			return c
		}
	}
	return p
}

// hasValueAt reports whether value sets a value at the path segments of a write-only property. A "*" segment matches
// every item of an array.
func hasValueAt(value any, segments []string) bool {
	if len(segments) == 0 {
		return true
	}
	switch v := value.(type) {
	case map[string]any:
		child, ok := v[segments[0]]
		return ok && hasValueAt(child, segments[1:])
	case []any:
		return segments[0] == "*" && slices.ContainsFunc(v, func(item any) bool { return hasValueAt(item, segments[1:]) })
	default:
		return false
	}
}

// valueAt returns the value at a "/"-separated path of nested objects in properties.
func valueAt(properties map[string]any, path string) (any, bool) {
	var current any = properties
	for _, segment := range strings.Split(path, "/") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		if current, ok = obj[segment]; !ok {
			return nil, false
		}
	}
	return current, true
}

// compareFingerprints compares the fingerprints of this deploy with the record's, and returns the status of each
// path. When the record has no fingerprints, or a different key made them, every path is unknown. Otherwise a path
// the record has no fingerprint for is changed, because the last deploy did not set it.
func compareFingerprints(previous *datamodel.AWSFingerprints, current *datamodel.AWSFingerprints) map[string]fingerprintStatus {
	out := map[string]fingerprintStatus{}
	if current == nil {
		return out
	}
	for path, digest := range current.Digests {
		switch {
		case previous == nil || previous.KeyID != current.KeyID:
			out[path] = fingerprintUnknown
		case hmac.Equal([]byte(previous.Digests[path]), []byte(digest)):
			out[path] = fingerprintUnchanged
		default:
			out[path] = fingerprintChanged
		}
	}
	return out
}

// withoutUnsentWriteOnly returns the patch without the write-only values UCP does not send:
//
//   - a value whose fingerprint matches, because it did not change
//   - a value with no usable fingerprint that cannot change after create, because UCP cannot tell whether it changed,
//     and sending it would fail the update. Every value of a create-and-delete-only type cannot change after create.
//
// A value with no usable fingerprint that can change, such as a password, stays in the patch, so it is sent once.
//
// When AWS omits an object because all of its properties are write-only, the patch adds the whole object in one
// operation. Its values that are not sent are removed from the operation, and the operation is dropped when nothing is
// left.
func withoutUnsentWriteOnly(patch jsondiff.Patch, schema *typeSchema, immutable bool, status map[string]fingerprintStatus) jsondiff.Patch {
	paths := fingerprintPaths(awsoperations.PropertyPaths(schema.WriteOnlyProperties))
	out := jsondiff.Patch{}
	for _, op := range patch {
		if op.Type != jsondiff.OperationAdd && op.Type != jsondiff.OperationReplace {
			out = append(out, op)
			continue
		}
		unsent := map[string]bool{}
		for _, leaf := range leafPaths(op.Path, op.Value) {
			if schema.isUnsent(leaf, paths, immutable, status) {
				unsent[leaf] = true
			}
		}
		if len(unsent) == 0 {
			out = append(out, op)
			continue
		}
		if value, kept := withoutPaths(op.Value, op.Path, unsent); kept {
			op.Value = value
			out = append(out, op)
		}
	}
	return out
}

// isUnsent reports whether the value at a JSON patch path is a write-only value UCP does not send. The status of a
// write-only property is the status of the fingerprint in paths that covers it.
func (s *typeSchema) isUnsent(patchPath string, paths []string, immutable bool, status map[string]fingerprintStatus) bool {
	pattern, ok := s.writeOnlyPattern(patchPath)
	if !ok {
		return false
	}
	switch status[coveringPath(paths, fingerprintPath(pattern))] {
	case fingerprintUnchanged:
		return true
	case fingerprintUnknown:
		return immutable || s.isCreateOnly(pattern)
	default:
		return false
	}
}

// withoutPaths returns value, which is at the JSON patch path path, without the values at the paths in remove, and
// whether anything is left. An object left empty is removed too. Array items keep their place, so an object item left
// with nothing stays as an empty object.
func withoutPaths(value any, path string, remove map[string]bool) (any, bool) {
	if remove[path] {
		return value, false
	}
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			return v, true
		}
		escape := strings.NewReplacer("~", "~0", "/", "~1")
		out := map[string]any{}
		for key, child := range v {
			if pruned, kept := withoutPaths(child, path+"/"+escape.Replace(key), remove); kept {
				out[key] = pruned
			}
		}
		return out, len(out) > 0
	case []any:
		if len(v) == 0 {
			return v, true
		}
		out := make([]any, len(v))
		kept := false
		for i, child := range v {
			pruned, ok := withoutPaths(child, fmt.Sprintf("%s/%d", path, i), remove)
			out[i] = pruned
			kept = kept || ok
		}
		return out, kept
	default:
		return value, true
	}
}

// isCreateOnly reports whether AWS cannot change a write-only property, such as Users/*/Secret, after create: it is
// create-only, inside a create-only property, or holds one.
func (s *typeSchema) isCreateOnly(writeOnly string) bool {
	return slices.ContainsFunc(awsoperations.PropertyPaths(s.CreateOnlyProperties), func(c string) bool {
		return c == writeOnly || strings.HasPrefix(writeOnly, c+"/") || strings.HasPrefix(c, writeOnly+"/")
	})
}
