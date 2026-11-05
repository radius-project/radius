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

package operations

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_GeneratePatchWithOptions_Equivalent(t *testing.T) {
	t.Parallel()

	current := map[string]any{"EngineVersion": "5.0.0", "Config": map[string]any{"Version": "2.1.0", "Mode": "fast"}}
	sameVersion := func(path string, template any, aws any) bool {
		return (path == "EngineVersion" || path == "Config/Version") && aws == template.(string)+".0"
	}
	tests := []struct {
		name       string
		desired    map[string]any
		equivalent func(path string, template any, aws any) bool
		want       int
	}{
		{
			name:       "values the rule matches are unchanged",
			desired:    map[string]any{"EngineVersion": "5.0", "Config": map[string]any{"Version": "2.1", "Mode": "fast"}},
			equivalent: sameVersion,
			want:       0,
		},
		{
			name:       "a value the rule doesn't match is a change",
			desired:    map[string]any{"EngineVersion": "6.0", "Config": map[string]any{"Version": "2.1", "Mode": "fast"}},
			equivalent: sameVersion,
			want:       1,
		},
		{
			name:    "without a rule, another form is a change",
			desired: map[string]any{"EngineVersion": "5.0", "Config": map[string]any{"Version": "2.1", "Mode": "fast"}},
			want:    2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentState, err := json.Marshal(current)
			require.NoError(t, err)
			desiredState, err := json.Marshal(tt.desired)
			require.NoError(t, err)
			patch, err := GeneratePatchWithOptions(currentState, desiredState, []byte(`{}`), PatchOptions{Equivalent: tt.equivalent})
			require.NoError(t, err)
			require.Len(t, patch, tt.want)
		})
	}
}
