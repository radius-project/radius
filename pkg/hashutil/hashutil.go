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

// Package hashutil provides deterministic content hashing used across Radius to
// generate stable identifiers and change-detection tokens (for example resource
// names, ETags, and configuration hashes).
//
// These hashes are NOT used for cryptographic or security purposes. They exist
// only to produce unique, deterministic values and to detect whether content has
// changed.
//
// New values must be produced with Hex (SHA-256). The legacy SHA-1 helper exists
// only so that Radius can recognize values written by older versions while they
// are migrated to SHA-256; see https://github.com/radius-project/radius/issues/8084.
package hashutil

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hex returns the SHA-256 hash of data encoded as a lowercase hexadecimal string.
//
// This is the hashing function that should be used for all new values.
func Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
