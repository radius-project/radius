/*
Copyright 2026 The Radius Authors.

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

package dynamicrp

import (
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
)

func Test_LoadConfig_AuthorizationMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    authz.Mode
		wantErr bool
	}{
		{name: "missing defaults to off", input: "logging:\n  level: info\n", want: authz.ModeOff},
		{name: "off", input: "authorization:\n  mode: off\n", want: authz.ModeOff},
		{name: "dryRun", input: "authorization:\n  mode: dryRun\n", want: authz.ModeDryRun},
		{name: "enforce", input: "authorization:\n  mode: enforce\n", want: authz.ModeEnforce},
		{name: "invalid", input: "authorization:\n  mode: audit\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config, err := LoadConfig([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, config.Authorization.EffectiveMode())
		})
	}
}
