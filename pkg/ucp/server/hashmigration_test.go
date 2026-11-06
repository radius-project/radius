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

package server

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/trackedresource"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
	"github.com/stretchr/testify/require"
)

type recordingReporter struct {
	statuses map[string]hashmigration.Status
}

func (r *recordingReporter) Report(_ context.Context, key string, status hashmigration.Status) error {
	r.statuses[key] = status
	return nil
}

type migratingClient struct {
	*inmemory.Client
	result hashmigration.Result
	err    error
	calls  int
}

func (c *migratingClient) MigrateLegacyNames(context.Context) (hashmigration.Result, error) {
	c.calls++
	return c.result, c.err
}

func Test_HashMigrationService(t *testing.T) {
	t.Parallel()
	group := "/planes/radius/local/resourceGroups/test-group"
	original := resources.MustParse(group + "/providers/Applications.Test/testResources/my-resource")

	t.Run("re-keys tracked resources; the data store step does not apply to other providers", func(t *testing.T) {
		t.Parallel()
		client := inmemory.NewClient()
		require.NoError(t, client.Save(t.Context(), &database.Object{ID: group, Data: map[string]any{"name": "test-group"}}))
		entry := datamodel.GenericResourceFromID(original, trackedresource.LegacyIDFor(original))
		entry.AsyncProvisioningState = v1.ProvisioningStateSucceeded
		require.NoError(t, client.Save(t.Context(), &database.Object{ID: trackedresource.LegacyIDFor(original).String(), Data: entry}))

		reporter := &recordingReporter{statuses: map[string]hashmigration.Status{}}
		require.True(t, newHashMigrationService(databaseprovider.FromClient(client), reporter).RunPass(t.Context()))

		tracked := reporter.statuses["ucp.trackedResources"]
		require.Equal(t, hashmigration.StateComplete, tracked.State)
		require.Equal(t, 1, tracked.Migrated)
		require.Equal(t, hashmigration.StateNotApplicable, reporter.statuses["ucp.datastore"].State)

		_, err := client.Get(t.Context(), trackedresource.IDFor(original).String())
		require.NoError(t, err)
		_, err = client.Get(t.Context(), trackedresource.LegacyIDFor(original).String())
		require.ErrorIs(t, err, &database.ErrNotFound{})
	})

	t.Run("runs the data store step for providers that derive names from hashes", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name     string
			result   hashmigration.Result
			err      error
			expected hashmigration.State
			done     bool
		}{
			{name: "complete", result: hashmigration.Result{Migrated: 2}, expected: hashmigration.StateComplete, done: true},
			{name: "remaining", result: hashmigration.Result{Remaining: 1}, expected: hashmigration.StateInProgress},
			{name: "error", err: errors.New("list failed"), expected: hashmigration.StateFailed},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				client := &migratingClient{Client: inmemory.NewClient(), result: tt.result, err: tt.err}
				reporter := &recordingReporter{statuses: map[string]hashmigration.Status{}}
				require.Equal(t, tt.done, newHashMigrationService(databaseprovider.FromClient(client), reporter).RunPass(t.Context()))
				require.Equal(t, 1, client.calls)
				require.Equal(t, tt.expected, reporter.statuses["ucp.datastore"].State)
				require.Equal(t, tt.result.Migrated, reporter.statuses["ucp.datastore"].Migrated)
			})
		}
	})

	t.Run("reports an unavailable database as a failure", func(t *testing.T) {
		t.Parallel()
		provider := databaseprovider.FromOptions(databaseprovider.Options{Provider: "unsupported"})
		reporter := &recordingReporter{statuses: map[string]hashmigration.Status{}}
		require.False(t, newHashMigrationService(provider, reporter).RunPass(t.Context()))
		require.Equal(t, hashmigration.StateFailed, reporter.statuses["ucp.trackedResources"].State)
		require.Equal(t, hashmigration.StateFailed, reporter.statuses["ucp.datastore"].State)
	})
}

func Test_NewServer_HashMigration(t *testing.T) {
	t.Parallel()
	for _, disabled := range []bool{false, true} {
		options := &ucp.Options{
			Config:           &ucp.Config{SHA256Migration: hashmigration.Config{Disabled: disabled}},
			DatabaseProvider: databaseprovider.FromMemory(),
		}
		host, err := NewServer(t.Context(), options)
		require.NoError(t, err)

		found := false
		for _, service := range host.Services {
			if _, ok := service.(*hashmigration.Service); ok {
				found = true
			}
		}
		require.Equal(t, !disabled, found)
	}
}

func Test_LoadConfig_SHA256Migration(t *testing.T) {
	t.Parallel()
	config, err := ucp.LoadConfig([]byte("sha256Migration:\n  disabled: true\n"))
	require.NoError(t, err)
	require.True(t, config.SHA256Migration.Disabled)

	config, err = ucp.LoadConfig([]byte("server:\n  port: 9443\n"))
	require.NoError(t, err)
	require.False(t, config.SHA256Migration.Disabled, "the migration must be enabled unless explicitly disabled")
}
