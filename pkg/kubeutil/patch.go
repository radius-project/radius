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

package kubeutil

import (
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"k8s.io/apimachinery/pkg/runtime"
)

// MergePatchObject merge-patches from old to cur with RFC7396 JSON merge patch
// and assign the patched object to out. It returns an error if the patch fails.
func MergePatchObject[T runtime.Object](old, cur, out T) error {
	oldBytes, err := json.Marshal(old)
	if err != nil {
		return fmt.Errorf("failed to marshal old object: %v", err)
	}

	newBytes, err := json.Marshal(cur)
	if err != nil {
		return fmt.Errorf("failed to marshal new object: %v", err)
	}

	patchBytes, err := jsonpatch.MergePatch(oldBytes, newBytes)
	if err != nil {
		return fmt.Errorf("failed to create patch: %v", err)
	}

	return json.Unmarshal(patchBytes, out)
}
