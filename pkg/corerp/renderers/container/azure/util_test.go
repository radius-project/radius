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

package azure

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMakeResourceName(t *testing.T) {
	nameTests := []struct {
		prefix string
		name   string
		out    string
	}{
		{
			"",
			"resource",
			"resource",
		},
		{
			"app",
			"resource",
			"app-resource",
		},
		{
			"app",
			"Resource",
			"app-resource",
		},
	}

	for _, tt := range nameTests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.out, MakeResourceName(tt.prefix, tt.name, Separator))
		})
	}
}
