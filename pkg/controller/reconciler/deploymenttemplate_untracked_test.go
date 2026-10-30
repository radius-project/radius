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

package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_isTrackedOutputResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id      string
		tracked bool
	}{
		{id: "/planes/radius/local/resourceGroups/g/providers/Radius.Compute/containers/vote", tracked: true},
		{id: "/planes/radius/local/resourceGroups/g/providers/Radius.Data/postgreSqlDatabases/db", tracked: true},
		{id: "/planes/radius/local/resourceGroups/g/providers/Applications.Core/applications/app", tracked: true},
		{id: "/planes/radius/local/resourceGroups/g/providers/Radius.Core/applications/app", tracked: false},
		{id: "/planes/radius/local/resourceGroups/g/providers/radius.core/environments/env", tracked: false},
		{id: "not-an-id", tracked: true},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.tracked, isTrackedOutputResource(tt.id))
		})
	}
}

func Test_untrackedDeleteOrder(t *testing.T) {
	t.Parallel()

	const p = "/planes/radius/local/resourceGroups/g/providers/"
	got := untrackedDeleteOrder([]string{
		p + "Radius.Core/recipePacks/pack",
		p + "Radius.Compute/containers/vote",
		p + "Radius.Core/environments/env",
		p + "Radius.Core/applications/app",
	})
	require.Equal(t, []string{
		p + "Radius.Core/applications/app",
		p + "Radius.Core/environments/env",
		p + "Radius.Core/recipePacks/pack",
	}, got)
}
