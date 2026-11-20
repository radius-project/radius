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

package aws

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_NewFingerprintKey(t *testing.T) {
	t.Parallel()

	t.Run("rejects keys shorter than the minimum", func(t *testing.T) {
		t.Parallel()
		for _, length := range []int{0, 1, MinFingerprintKeyLength - 1} {
			key, err := NewFingerprintKey(make([]byte, length))
			require.Nil(t, key)
			require.ErrorContains(t, err, "must be at least 32 bytes")
		}
	})

	t.Run("accepts keys of the minimum length or longer", func(t *testing.T) {
		t.Parallel()
		for _, length := range []int{MinFingerprintKeyLength, 64} {
			key, err := NewFingerprintKey(bytes.Repeat([]byte{7}, length))
			require.NoError(t, err)
			require.NotNil(t, key)
		}
	})

	t.Run("ID is stable for a key and differs between keys", func(t *testing.T) {
		t.Parallel()
		raw := bytes.Repeat([]byte{1}, MinFingerprintKeyLength)
		first, err := NewFingerprintKey(raw)
		require.NoError(t, err)
		again, err := NewFingerprintKey(bytes.Clone(raw))
		require.NoError(t, err)
		other, err := NewFingerprintKey(bytes.Repeat([]byte{2}, MinFingerprintKeyLength))
		require.NoError(t, err)

		require.Regexp(t, regexp.MustCompile(`^[0-9a-f]{12}$`), first.ID())
		require.Equal(t, first.ID(), again.ID())
		require.NotEqual(t, first.ID(), other.ID())
	})

	t.Run("ID format does not change", func(t *testing.T) {
		t.Parallel()
		// Records keep the ID of the key that made each fingerprint, so the ID of a key must stay the same across
		// UCP versions.
		raw := make([]byte, MinFingerprintKeyLength)
		for i := range raw {
			raw[i] = byte(i)
		}
		key, err := NewFingerprintKey(raw)
		require.NoError(t, err)
		require.Equal(t, "db8bb34ebcef", key.ID())
	})

	t.Run("NewHMAC is HMAC-SHA-256 with the key", func(t *testing.T) {
		t.Parallel()
		raw := bytes.Repeat([]byte{3}, MinFingerprintKeyLength)
		key, err := NewFingerprintKey(raw)
		require.NoError(t, err)

		want := hmac.New(sha256.New, raw)
		want.Write([]byte("value"))
		got := key.NewHMAC()
		got.Write([]byte("value"))
		require.Equal(t, want.Sum(nil), got.Sum(nil))

		// Each call returns a fresh hash.
		fresh := key.NewHMAC()
		fresh.Write([]byte("value"))
		require.Equal(t, want.Sum(nil), fresh.Sum(nil))
	})

	t.Run("keeps its own copy of the key", func(t *testing.T) {
		t.Parallel()
		raw := bytes.Repeat([]byte{4}, MinFingerprintKeyLength)
		key, err := NewFingerprintKey(raw)
		require.NoError(t, err)
		id := key.ID()
		before := key.NewHMAC().Sum(nil)

		raw[0] = 5

		require.Equal(t, id, key.ID())
		require.Equal(t, before, key.NewHMAC().Sum(nil))
	})
}

func Test_LoadFingerprintKey(t *testing.T) {
	t.Parallel()

	t.Run("loads the key from the file", func(t *testing.T) {
		t.Parallel()
		raw := bytes.Repeat([]byte{9}, MinFingerprintKeyLength)
		path := filepath.Join(t.TempDir(), "key")
		require.NoError(t, os.WriteFile(path, raw, 0400))

		key, err := LoadFingerprintKey(path)
		require.NoError(t, err)
		want, err := NewFingerprintKey(raw)
		require.NoError(t, err)
		require.Equal(t, want.ID(), key.ID())
	})

	t.Run("missing file names the Secret", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "key")

		key, err := LoadFingerprintKey(path)
		require.Nil(t, key)
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.ErrorContains(t, err, FingerprintKeySecretName)
		require.ErrorContains(t, err, path)
	})

	t.Run("unreadable path names the Secret", func(t *testing.T) {
		t.Parallel()
		path := t.TempDir()

		key, err := LoadFingerprintKey(path)
		require.Nil(t, key)
		require.ErrorContains(t, err, "failed to read the AWS fingerprint key from the Kubernetes Secret "+FingerprintKeySecretName)
	})

	t.Run("short key names the Secret", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "key")
		require.NoError(t, os.WriteFile(path, []byte("s3cr3t-v4lue"), 0400))

		key, err := LoadFingerprintKey(path)
		require.Nil(t, key)
		require.ErrorContains(t, err, "invalid AWS fingerprint key in the Kubernetes Secret "+FingerprintKeySecretName)
		require.ErrorContains(t, err, "is 12 bytes")
		require.NotContains(t, err.Error(), "s3cr3t-v4lue", "Errors must not include the key.")
	})
}

func Test_FingerprintKey_Fingerprint(t *testing.T) {
	t.Parallel()

	raw := make([]byte, MinFingerprintKeyLength)
	for i := range raw {
		raw[i] = byte(i)
	}
	key, err := NewFingerprintKey(raw)
	require.NoError(t, err)
	other, err := NewFingerprintKey(bytes.Repeat([]byte{9}, MinFingerprintKeyLength))
	require.NoError(t, err)

	const id = "/planes/aws/aws/accounts/1/regions/us-west-2/providers/AWS.RDS/DBInstance/app-db"
	fingerprint := func(t *testing.T, key *FingerprintKey, resourceID string, path string, value string) string {
		t.Helper()
		var decoded any
		require.NoError(t, json.Unmarshal([]byte(value), &decoded))
		got, err := key.Fingerprint(resourceID, path, decoded)
		require.NoError(t, err)
		return got
	}
	base := fingerprint(t, key, id, "MasterUserPassword", `"p@ss"`)

	t.Run("encoding does not change", func(t *testing.T) {
		t.Parallel()
		// Records keep fingerprints across UCP versions, so the same input must always give the same fingerprint. The
		// expected value was computed separately with Python's hmac module.
		require.Equal(t, "9kZDs5tXgt65jiYHO8+2xf2+bqxvmHBe1Y4TXJUnBJw", base)

		mac := hmac.New(sha256.New, raw)
		for _, input := range []string{"radius-aws-write-only-fingerprint/v1", strings.ToLower(id), "MasterUserPassword", `"p@ss"`} {
			mac.Write(binary.BigEndian.AppendUint64(nil, uint64(len(input))))
			mac.Write([]byte(input))
		}
		require.Equal(t, base64.RawStdEncoding.EncodeToString(mac.Sum(nil)), base)
	})

	t.Run("equal values have the same fingerprint", func(t *testing.T) {
		t.Parallel()
		a := fingerprint(t, key, id, "Auth", `{"b": 1, "a": [2.0, "x", null, true]}`)
		b := fingerprint(t, key, id, "Auth", `{"a": [2, "x", null, true], "b": 1e0}`)
		require.Equal(t, a, b)
		require.Equal(t, base, fingerprint(t, key, strings.ToUpper(id), "MasterUserPassword", `"p@ss"`), "resource IDs are compared ignoring case")
	})

	t.Run("anything else changes the fingerprint", func(t *testing.T) {
		t.Parallel()
		different := map[string]string{
			"value":       fingerprint(t, key, id, "MasterUserPassword", `"p@ss2"`),
			"value type":  fingerprint(t, key, id, "MasterUserPassword", `["p@ss"]`),
			"resource":    fingerprint(t, key, id+"-2", "MasterUserPassword", `"p@ss"`),
			"path":        fingerprint(t, key, id, "TdeCredentialPassword", `"p@ss"`),
			"key":         fingerprint(t, other, id, "MasterUserPassword", `"p@ss"`),
			"path prefix": fingerprint(t, key, id, "MasterUserPasswor", `"dp@ss"`),
		}
		seen := map[string]string{base: "base"}
		for name, got := range different {
			require.NotContains(t, seen, got, "%s gives the same fingerprint as %s", name, seen[got])
			seen[got] = name
		}
	})

	t.Run("a value that is not JSON fails", func(t *testing.T) {
		t.Parallel()
		_, err := key.Fingerprint(id, "Port", math.Inf(1))
		require.ErrorContains(t, err, "failed to encode the value of Port for its fingerprint")
	})
}
