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

package recipes

import (
	"fmt"
	"sort"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

// ResolveSecretWriteBacks turns a recipe definition's SecretOutputs mapping into concrete write-back
// instructions (the "backwards" secret flow). For each entry — keyed by the name of a
// Radius.Security/secrets resource the deploying resource binds via an x-radius-secret-binding array
// property — it matches the bound secret ID (by name) from secretBindings and pairs each declared secret
// data key with the value of the named module output (looked up in secrets first, then values). It returns
// the resolved write-backs plus the set of module output names consumed, so the driver can drop them from
// the plain recipe response (a value written into a secret must not also be surfaced as an output).
//
// It operates on the module's RAW outputs (before any `outputs` mapping is applied), because that mapping
// keeps only explicitly mapped outputs — a secure output routed exclusively into a secret would otherwise
// be dropped before reaching the caller. It is fail-closed: a SecretOutputs entry naming a secret the
// resource does not bind, or a module output the recipe did not produce, is an error rather than a silently
// unpopulated secret.
func ResolveSecretWriteBacks(values map[string]any, secrets map[string]any, secretOutputs map[string]map[string]string, secretBindings []string) ([]SecretWriteBack, map[string]struct{}, error) {
	if len(secretOutputs) == 0 {
		return nil, nil, nil
	}

	// Index the resource's bound secret IDs by resource name so a SecretOutputs entry (keyed by secret
	// name) resolves to a fully qualified secret ID.
	boundByName := map[string]string{}
	for _, secretID := range secretBindings {
		if secretID == "" {
			continue
		}
		parsed, err := resources.ParseResource(secretID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid bound secret ID %q: %w", secretID, err)
		}
		boundByName[parsed.Name()] = secretID
	}

	// Sort secret names for deterministic ordering.
	secretNames := make([]string, 0, len(secretOutputs))
	for name := range secretOutputs {
		secretNames = append(secretNames, name)
	}
	sort.Strings(secretNames)

	writeBacks := make([]SecretWriteBack, 0, len(secretNames))
	consumed := map[string]struct{}{}

	for _, secretName := range secretNames {
		secretID, ok := boundByName[secretName]
		if !ok {
			return nil, nil, fmt.Errorf("recipe declares secretOutputs for secret %q but the resource does not bind a Radius.Security/secrets named %q (add it to the resource's secrets array)", secretName, secretName)
		}

		keyMap := secretOutputs[secretName]

		// Sort data keys for deterministic ordering.
		dataKeys := make([]string, 0, len(keyMap))
		for k := range keyMap {
			dataKeys = append(dataKeys, k)
		}
		sort.Strings(dataKeys)

		data := make(map[string]string, len(keyMap))
		for _, dataKey := range dataKeys {
			outputName := keyMap[dataKey]
			val, ok := secrets[outputName]
			if !ok {
				val, ok = values[outputName]
			}
			if !ok {
				return nil, nil, fmt.Errorf("recipe declares secretOutputs %s.%s but the module produced no output named %q", secretName, dataKey, outputName)
			}

			strVal, ok := val.(string)
			if !ok {
				strVal = fmt.Sprintf("%v", val)
			}
			data[dataKey] = strVal
			consumed[outputName] = struct{}{}
		}

		writeBacks = append(writeBacks, SecretWriteBack{SecretID: secretID, Data: data})
	}

	return writeBacks, consumed, nil
}
