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

package handlers

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
)

// Test_NewRegistry checks that every registered handler has a schema built into Radius.
func Test_NewRegistry(t *testing.T) {
	registry, err := NewRegistry(aws.Config{})
	require.NoError(t, err)
	for _, h := range All(aws.Config{}) {
		require.NotNil(t, registry.Lookup(h.TypeName()), h.TypeName())
	}
}
