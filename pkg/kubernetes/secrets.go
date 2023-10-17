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
	"crypto/sha1"
	"fmt"
	"sort"
)

// HashSecretData hashes the data in a secret to produce a deterministic hash.
//
// This can be used as a Kubernetes annotation to force a Deployment to redeploy pods
// when the secret changes.
func HashSecretData(secretData map[string][]byte) string {
	// Sort keys so we can hash deterministically
	keys := []string{}
	for k := range secretData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	hash := sha1.New()

	for _, k := range keys {
		// Using | as a delimiter
		_, _ = hash.Write([]byte("|" + k + "|"))
		_, _ = hash.Write(secretData[k])
	}

	sum := hash.Sum(nil)
	return fmt.Sprintf("%x", sum)
}
