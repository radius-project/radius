/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeString(t *testing.T) {
	testrt := []struct {
		in  string
		out string
	}{
		{"applications.core/environments", "applicationscore-environments"},
		{"applications.core/provider", "applicationscore-provider"},
		{"applications.link/provider", "applicationslink-provider"},
	}

	for _, tc := range testrt {
		t.Run(tc.in, func(t *testing.T) {
			normalized := NormalizeStringToLower(tc.in)
			require.Equal(t, tc.out, normalized)
		})
	}
}
