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

package kubernetes

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex" //nolint:gosec // Computes the legacy secret hash independently to pin its format.
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_HashSecretData(t *testing.T) {
	t.Parallel()

	data := map[string][]byte{"b": []byte("2"), "a": []byte("1")}
	hash := HashSecretData(data)
	require.True(t, strings.HasPrefix(hash, SecretHashPrefix))
	require.Len(t, hash, len(SecretHashPrefix)+64)
	require.False(t, IsLegacySecretHash(hash))
	// Pin the format: SHA-256 over length-prefixed fields, starting with the domain.
	encoded := []byte{}
	for _, field := range []string{secretHashDomain, "a", "1", "b", "2"} {
		encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(field)))
		encoded = append(encoded, field...)
	}
	sum := sha256.Sum256(encoded)
	require.Equal(t, SecretHashPrefix+hex.EncodeToString(sum[:]), hash)

	// The hash is deterministic and independent of map iteration order.
	require.Equal(t, hash, HashSecretData(map[string][]byte{"a": []byte("1"), "b": []byte("2")}))

	// Any change to a key or value changes the hash.
	require.NotEqual(t, hash, HashSecretData(map[string][]byte{"a": []byte("1"), "b": []byte("3")}))
	require.NotEqual(t, hash, HashSecretData(map[string][]byte{"a": []byte("1"), "c": []byte("2")}))
	require.NotEqual(t, hash, HashSecretData(map[string][]byte{"a": []byte("1")}))

	// The encoding is unambiguous, unlike the legacy delimiter-based encoding.
	require.Equal(t,
		LegacyHashSecretData(map[string][]byte{"a": []byte("1|b|2")}),
		LegacyHashSecretData(map[string][]byte{"a": []byte("1"), "b": []byte("2")}))
	require.NotEqual(t,
		HashSecretData(map[string][]byte{"a": []byte("1|b|2")}),
		HashSecretData(map[string][]byte{"a": []byte("1"), "b": []byte("2")}))
	require.NotEqual(t, HashSecretData(map[string][]byte{"ab": nil}), HashSecretData(map[string][]byte{"a": []byte("b")}))
}

func Test_LegacyHashSecretData(t *testing.T) {
	t.Parallel()

	// The legacy hash must match what older versions of Radius wrote into Pod templates.
	data := map[string][]byte{"b": []byte("2"), "a": []byte("1")}
	expected := fmt.Sprintf("%x", sha1.Sum([]byte("|a|1|b|2"))) //nolint:gosec // Pins the legacy format.
	require.Equal(t, expected, LegacyHashSecretData(data))
	require.True(t, IsLegacySecretHash(LegacyHashSecretData(data)))
	require.Equal(t, fmt.Sprintf("%x", sha1.Sum(nil)), LegacyHashSecretData(nil)) //nolint:gosec // Pins the legacy format.
}

func Test_IsLegacySecretHash(t *testing.T) {
	t.Parallel()

	for value, expected := range map[string]bool{
		"0123456789abcdef0123456789abcdef01234567":  true,
		"0123456789ABCDEF0123456789abcdef01234567":  false,
		"0123456789abcdef0123456789abcdef0123456":   false,
		"0123456789abcdef0123456789abcdef012345678": false,
		"0123456789abcdef0123456789abcdef0123456g":  false,
		"": false,
		SecretHashPrefix + strings.Repeat("0", 64):        false,
		"sha256:0123456789abcdef0123456789abcdef01234567": false,
	} {
		require.Equal(t, expected, IsLegacySecretHash(value), value)
	}
}
