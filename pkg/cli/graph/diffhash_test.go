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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeDiffHash_DeterministicOutput(t *testing.T) {
	t.Parallel()

	props := map[string]interface{}{
		"connections": map[string]interface{}{
			"backend": map[string]interface{}{"source": "someId"},
		},
		"container": map[string]interface{}{
			"image": "myregistry/frontend:latest",
		},
	}

	hash1 := ComputeDiffHash(props)
	hash2 := ComputeDiffHash(props)

	assert.NotEmpty(t, hash1)
	assert.Equal(t, hash1, hash2, "same input should produce same hash")
}

func TestComputeDiffHash_DifferentPropertiesProduceDifferentHash(t *testing.T) {
	t.Parallel()

	props1 := map[string]interface{}{
		"container": map[string]interface{}{
			"image": "myregistry/frontend:v1",
		},
	}

	props2 := map[string]interface{}{
		"container": map[string]interface{}{
			"image": "myregistry/frontend:v2",
		},
	}

	hash1 := ComputeDiffHash(props1)
	hash2 := ComputeDiffHash(props2)

	assert.NotEmpty(t, hash1)
	assert.NotEmpty(t, hash2)
	assert.NotEqual(t, hash1, hash2, "different inputs should produce different hashes")
}

func TestComputeDiffHash_StableAcrossMapIteration(t *testing.T) {
	t.Parallel()

	// Run multiple times to catch map iteration ordering issues.
	props := map[string]interface{}{
		"connections": map[string]interface{}{
			"alpha": map[string]interface{}{"source": "a"},
			"beta":  map[string]interface{}{"source": "b"},
			"gamma": map[string]interface{}{"source": "c"},
		},
		"container": map[string]interface{}{
			"image": "myregistry/app:latest",
			"ports": map[string]interface{}{
				"http": map[string]interface{}{"containerPort": float64(3000)},
				"grpc": map[string]interface{}{"containerPort": float64(50051)},
			},
		},
	}

	first := ComputeDiffHash(props)
	for i := 0; i < 20; i++ {
		assert.Equal(t, first, ComputeDiffHash(props), "hash must be stable across iterations")
	}
}

func TestComputeDiffHash_IgnoresNonRelevantProperties(t *testing.T) {
	t.Parallel()

	props1 := map[string]interface{}{
		"container": map[string]interface{}{
			"image": "myregistry/app:v1",
		},
		"name":        "myapp",
		"application": "someAppId",
	}

	props2 := map[string]interface{}{
		"container": map[string]interface{}{
			"image": "myregistry/app:v1",
		},
		"name":        "differentName",
		"application": "differentAppId",
	}

	assert.Equal(t, ComputeDiffHash(props1), ComputeDiffHash(props2),
		"non-review-relevant properties should not affect hash")
}

func TestComputeDiffHash_EmptyProperties(t *testing.T) {
	t.Parallel()

	hash := ComputeDiffHash(map[string]interface{}{})
	assert.NotEmpty(t, hash, "empty properties should still produce a hash")
}
