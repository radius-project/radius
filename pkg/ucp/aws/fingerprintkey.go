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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"slices"
	"strings"
)

const (
	// FingerprintKeySecretName is the Kubernetes Secret that holds the HMAC key for fingerprints of write-only AWS
	// property values. The Helm chart creates it and mounts it read-only into UCP.
	FingerprintKeySecretName = "radius-aws-fingerprint-key" //nolint:gosec // This is a Secret name, not a credential.

	// DefaultFingerprintKeyFile is where the Helm chart mounts the key from FingerprintKeySecretName.
	DefaultFingerprintKeyFile = "/var/secrets/aws-fingerprint/key"

	// MinFingerprintKeyLength is the shortest HMAC key accepted, in bytes.
	MinFingerprintKeyLength = 32

	// fingerprintDomain is the first input of every fingerprint. It keeps fingerprints apart from any other use of the
	// key, and names the version of the encoding, so a later encoding cannot match a fingerprint made with this one.
	fingerprintDomain = "radius-aws-write-only-fingerprint/v1"
)

// FingerprintKey is the HMAC key UCP uses to fingerprint write-only AWS property values, such as passwords, so a
// redeploy can tell whether a value changed without storing it. The key is never written to the data store.
type FingerprintKey struct {
	key []byte
	id  string
}

// NewFingerprintKey creates a FingerprintKey from a key of at least MinFingerprintKeyLength bytes.
func NewFingerprintKey(key []byte) (*FingerprintKey, error) {
	if len(key) < MinFingerprintKeyLength {
		return nil, fmt.Errorf("the AWS fingerprint key is %d bytes; it must be at least %d bytes", len(key), MinFingerprintKeyLength)
	}

	// The ID is a hash of the key, so it identifies the key without revealing it.
	sum := sha256.Sum256(append([]byte(FingerprintKeySecretName+"\x00"), key...))
	return &FingerprintKey{key: slices.Clone(key), id: hex.EncodeToString(sum[:])[:12]}, nil
}

// LoadFingerprintKey reads the key from path. Errors name the Kubernetes Secret the Helm chart mounts the key from.
func LoadFingerprintKey(path string) (*FingerprintKey, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read the AWS fingerprint key from the Kubernetes Secret %s mounted at %s: %w", FingerprintKeySecretName, path, err)
	}

	k, err := NewFingerprintKey(key)
	if err != nil {
		return nil, fmt.Errorf("invalid AWS fingerprint key in the Kubernetes Secret %s mounted at %s: %w", FingerprintKeySecretName, path, err)
	}

	return k, nil
}

// ID returns an identifier for the key that does not reveal it. A fingerprint records the ID of the key that made
// it, so UCP can tell which fingerprints an earlier key made.
func (k *FingerprintKey) ID() string {
	return k.id
}

// NewHMAC returns a new HMAC-SHA-256 hash keyed with the key.
func (k *FingerprintKey) NewHMAC() hash.Hash {
	return hmac.New(sha256.New, k.key)
}

// Fingerprint returns the HMAC-SHA-256 of a write-only property value, base64-encoded. It is bound to the resource and
// to the property path, so the same value under another resource or property has a different fingerprint.
//
// The value must be a value decoded from JSON. It is encoded as JSON, which sorts object keys and writes each number
// in one form, so equal values always have the same fingerprint. Each input is prefixed with its length, so no two
// different inputs encode the same. resourceID is compared ignoring case, like Radius IDs.
func (k *FingerprintKey) Fingerprint(resourceID string, path string, value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to encode the value of %s for its fingerprint: %w", path, err)
	}

	mac := k.NewHMAC()
	for _, input := range [][]byte{[]byte(fingerprintDomain), []byte(strings.ToLower(resourceID)), []byte(path), encoded} {
		mac.Write(binary.BigEndian.AppendUint64(nil, uint64(len(input))))
		mac.Write(input)
	}
	return base64.RawStdEncoding.EncodeToString(mac.Sum(nil)), nil
}
