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

package graph

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeDiffHash_StableAcrossMapOrder(t *testing.T) {
	t.Parallel()

	props1 := map[string]any{"a": 1, "b": "two", "c": []any{1, 2, 3}}
	props2 := map[string]any{"c": []any{1, 2, 3}, "b": "two", "a": 1}

	hash1, err := ComputeDiffHash(props1)
	require.NoError(t, err)
	hash2, err := ComputeDiffHash(props2)
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(hash1, "sha256:"))
	require.Equal(t, hash1, hash2, "hash must be stable across map iteration order")
}

func TestComputeDiffHash_StableAcrossDependsOnOrder(t *testing.T) {
	t.Parallel()

	props := map[string]any{"a": 1}

	hash1, err := ComputeDiffHash(props, "id-b", "id-a", "id-c")
	require.NoError(t, err)
	hash2, err := ComputeDiffHash(props, "id-c", "id-a", "id-b")
	require.NoError(t, err)

	require.Equal(t, hash1, hash2)
}

func TestComputeDiffHash_IgnoresNonAuthorableProperties(t *testing.T) {
	t.Parallel()

	authored := map[string]any{"image": "nginx", "ports": []any{80}}
	withRuntime := map[string]any{
		"image":             "nginx",
		"ports":             []any{80},
		"provisioningState": "Succeeded",
		"status":            map[string]any{"phase": "Running"},
	}

	hash1, err := ComputeDiffHash(authored)
	require.NoError(t, err)
	hash2, err := ComputeDiffHash(withRuntime)
	require.NoError(t, err)

	require.Equal(t, hash1, hash2, "runtime-bound properties must not affect diff hash")
}

func TestComputeDiffHash_DifferentPropertiesProduceDifferentHashes(t *testing.T) {
	t.Parallel()

	hash1, err := ComputeDiffHash(map[string]any{"image": "nginx"})
	require.NoError(t, err)
	hash2, err := ComputeDiffHash(map[string]any{"image": "redis"})
	require.NoError(t, err)

	require.NotEqual(t, hash1, hash2)
}

func TestComputeDiffHash_DependsOnAffectsHash(t *testing.T) {
	t.Parallel()

	props := map[string]any{"image": "nginx"}

	hash1, err := ComputeDiffHash(props)
	require.NoError(t, err)
	hash2, err := ComputeDiffHash(props, "id-a")
	require.NoError(t, err)

	require.NotEqual(t, hash1, hash2)
}

func TestComputeDiffHash_EmptyInputs(t *testing.T) {
	t.Parallel()

	hash, err := ComputeDiffHash(nil)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hash, "sha256:"))
	require.Greater(t, len(hash), len("sha256:"))
}
