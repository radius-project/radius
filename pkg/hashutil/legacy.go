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

package hashutil

// This file centralizes the SHA-1 implementation behind pkg/hashutil. SHA-1 is used here
// only for non-cryptographic, backward-compatible reads of identifiers and change-detection
// tokens written by older versions of Radius while they are migrated to SHA-256.
//
// A few call sites outside hashutil still compute SHA-1 directly and are migrated in a later
// phase - notably the Kubernetes secret-hash pod annotation (pkg/kubernetes/secrets.go) and
// the ACI gateway DNS prefix (pkg/corerp/renderers/aci/gateway/render.go). Once those are
// migrated and all stored SHA-1 values have been re-keyed, deleting this file completes the
// migration away from SHA-1.
//
// See https://github.com/radius-project/radius/issues/8084.

import (
	"crypto/sha1"
	"encoding/hex"
)

// LegacyHex returns the SHA-1 hash of data encoded as a lowercase hexadecimal string.
//
// SHA-1 is cryptographically broken and is retained only so that Radius can recognize
// values written by older versions during the migration to SHA-256. Use Hex for all new
// values; do not use LegacyHex to produce new values.
func LegacyHex(data []byte) string {
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:])
}
