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

package kubernetes

import (
	"encoding/binary"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/hashutil"
)

const (
	// SecretHashPrefix starts every secret hash HashSecretData returns. Secret hashes that older versions of Radius
	// computed with SHA-1 have no prefix.
	SecretHashPrefix = "sha256:"

	// secretHashDomain separates secret hashes from other SHA-256 values computed over the same bytes.
	secretHashDomain = "radapp.io/secret-hash/v1"

	// legacySecretHashLength is the length of a hexadecimal SHA-1 hash.
	legacySecretHashLength = 40
)

// HashSecretData hashes the data in a secret to produce a deterministic hash.
//
// This can be used as a Kubernetes annotation to force a Deployment to redeploy pods
// when the secret changes. The hash is SHA-256 over an unambiguous encoding of the
// sorted keys and values, prefixed with SecretHashPrefix.
func HashSecretData(secretData map[string][]byte) string {
	encoded := appendSecretHashField(nil, []byte(secretHashDomain))
	for _, key := range sortedKeys(secretData) {
		encoded = appendSecretHashField(encoded, []byte(key))
		encoded = appendSecretHashField(encoded, secretData[key])
	}

	return SecretHashPrefix + hashutil.Hex(encoded)
}

// LegacyHashSecretData returns the secret hash that older versions of Radius computed with SHA-1.
//
// It exists only so Radius can tell whether a Pod template that carries a legacy hash still matches
// its secret, and keep that Pod template instead of rolling out the workload. Use HashSecretData for
// new values. See https://github.com/radius-project/radius/issues/8084.
func LegacyHashSecretData(secretData map[string][]byte) string {
	var encoded []byte
	for _, key := range sortedKeys(secretData) {
		// Older versions of Radius used | as a delimiter.
		encoded = append(encoded, '|')
		encoded = append(encoded, key...)
		encoded = append(encoded, '|')
		encoded = append(encoded, secretData[key]...)
	}

	return hashutil.LegacyHex(encoded)
}

// IsLegacySecretHash reports whether value has the form of a secret hash computed by LegacyHashSecretData.
func IsLegacySecretHash(value string) bool {
	if len(value) != legacySecretHashLength {
		return false
	}

	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}

// RecordSecretHash returns the AnnotationSecretHashSHA256 value that records current as the secret hash of the secret
// data that the legacy secret hash legacy was computed from.
func RecordSecretHash(legacy string, current string) string {
	return legacy + "=" + current
}

// ParseRecordedSecretHash returns the legacy secret hash and the current secret hash of the same secret data that an
// AnnotationSecretHashSHA256 value records.
func ParseRecordedSecretHash(value string) (legacy string, current string, ok bool) {
	legacy, current, ok = strings.Cut(value, "=")
	if !ok || !IsLegacySecretHash(legacy) || !strings.HasPrefix(current, SecretHashPrefix) {
		return "", "", false
	}

	return legacy, current, true
}

// appendSecretHashField appends value to encoded, preceded by its length, so that no two different
// sets of keys and values encode to the same bytes.
func appendSecretHashField(encoded []byte, value []byte) []byte {
	encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(value)))
	return append(encoded, value...)
}

func sortedKeys(secretData map[string][]byte) []string {
	keys := make([]string, 0, len(secretData))
	for key := range secretData {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
