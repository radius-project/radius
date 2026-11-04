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

package trackedresource

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/apiserverstore"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const migrateGroup = "/planes/radius/local/resourceGroups/test-group"

func newStores(t *testing.T) map[string]func() database.Client {
	t.Helper()
	return map[string]func() database.Client{
		"inmemory": func() database.Client { return inmemory.NewClient() },
		"apiserver": func() database.Client {
			scheme := runtime.NewScheme()
			require.NoError(t, ucpv1alpha1.AddToScheme(scheme))
			return apiserverstore.NewAPIServerClient(fake.NewClientBuilder().WithScheme(scheme).Build(), "radius-system")
		},
	}
}

func saveTracked(t *testing.T, client database.Client, original resources.ID, trackingID resources.ID, state v1.ProvisioningState) *database.Object {
	t.Helper()
	entry := datamodel.GenericResourceFromID(original, trackingID)
	entry.Properties.APIVersion = "2023-01-01"
	entry.AsyncProvisioningState = state
	obj := &database.Object{ID: trackingID.String(), Data: entry}
	require.NoError(t, client.Save(t.Context(), obj))
	return obj
}

func readTracked(t *testing.T, client database.Client, id resources.ID) *datamodel.GenericResource {
	t.Helper()
	obj, err := client.Get(t.Context(), id.String())
	if errors.Is(err, &database.ErrNotFound{}) {
		return nil
	}
	require.NoError(t, err)
	entry := &datamodel.GenericResource{}
	require.NoError(t, obj.As(entry))
	return entry
}

func Test_MigrateLegacyEntries(t *testing.T) {
	t.Parallel()
	original := resources.MustParse(migrateGroup + "/providers/Applications.Test/testResources/my-resource")
	currentID, legacyID := IDFor(original), LegacyIDFor(original)
	for storeName, newStore := range newStores(t) {
		for _, tt := range []struct {
			name          string
			current       v1.ProvisioningState
			legacy        v1.ProvisioningState
			hasCurrent    bool
			hasLegacy     bool
			expected      hashmigration.Result
			legacyRemains bool
		}{
			{name: "legacy idle", hasLegacy: true, legacy: v1.ProvisioningStateSucceeded, expected: hashmigration.Result{Migrated: 1}},
			{name: "legacy without state", hasLegacy: true, expected: hashmigration.Result{Migrated: 1}},
			{name: "legacy failed", hasLegacy: true, legacy: v1.ProvisioningStateFailed, expected: hashmigration.Result{Migrated: 1}},
			{name: "legacy busy", hasLegacy: true, legacy: v1.ProvisioningStateAccepted, expected: hashmigration.Result{Remaining: 1}, legacyRemains: true},
			{name: "current only", hasCurrent: true, current: v1.ProvisioningStateSucceeded},
			{name: "both idle", hasCurrent: true, hasLegacy: true, legacy: v1.ProvisioningStateSucceeded, current: v1.ProvisioningStateAccepted, expected: hashmigration.Result{Deduplicated: 1}},
			{name: "both with busy legacy", hasCurrent: true, hasLegacy: true, legacy: v1.ProvisioningStateUpdating, expected: hashmigration.Result{Remaining: 1}, legacyRemains: true},
		} {
			t.Run(storeName+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				client := newStore()
				require.NoError(t, client.Save(t.Context(), &database.Object{ID: migrateGroup, Data: map[string]any{"name": "test-group"}}))
				if tt.hasLegacy {
					saveTracked(t, client, original, legacyID, tt.legacy)
				}
				if tt.hasCurrent {
					saveTracked(t, client, original, currentID, tt.current)
				}

				result, err := MigrateLegacyEntries(t.Context(), client)
				require.NoError(t, err)
				require.Equal(t, tt.expected, result)

				legacyEntry := readTracked(t, client, legacyID)
				require.Equal(t, tt.legacyRemains, legacyEntry != nil)
				currentEntry := readTracked(t, client, currentID)
				if tt.hasCurrent || !tt.legacyRemains {
					require.NotNil(t, currentEntry)
					require.Equal(t, currentID.String(), currentEntry.ID)
					require.Equal(t, currentID.Name(), currentEntry.Name)
					require.Equal(t, original.String(), currentEntry.Properties.ID)
					require.Equal(t, "2023-01-01", currentEntry.Properties.APIVersion)
				} else {
					require.Nil(t, currentEntry, "a busy legacy entry must not be copied")
				}
				if tt.hasCurrent {
					require.Equal(t, tt.current, currentEntry.AsyncProvisioningState, "an existing current entry is never overwritten")
				} else if currentEntry != nil {
					require.Equal(t, tt.legacy, currentEntry.AsyncProvisioningState)
				}

				listed, err := client.Query(t.Context(), database.Query{RootScope: migrateGroup, ResourceType: datamodel.GenericResourceType})
				require.NoError(t, err)
				expectedCount := 1
				if tt.legacyRemains && tt.hasCurrent {
					expectedCount = 2
				}
				require.Len(t, listed.Items, expectedCount)

				again, err := MigrateLegacyEntries(t.Context(), client)
				require.NoError(t, err)
				require.Equal(t, hashmigration.Result{Remaining: tt.expected.Remaining}, again)
			})
		}
	}
}

func Test_MigrateLegacyEntries_SkipsUnrecognizedEntries(t *testing.T) {
	t.Parallel()
	client := inmemory.NewClient()
	require.NoError(t, client.Save(t.Context(), &database.Object{ID: migrateGroup, Data: map[string]any{}}))
	original := resources.MustParse(migrateGroup + "/providers/Applications.Test/testResources/my-resource")
	other := resources.MustParse(migrateGroup + "/providers/System.Resources/resources/unrelated-name")
	saveTracked(t, client, original, other, v1.ProvisioningStateSucceeded)

	invalid := datamodel.GenericResourceFromID(original, LegacyIDFor(original))
	invalid.Properties.ID = "not an id"
	require.NoError(t, client.Save(t.Context(), &database.Object{ID: LegacyIDFor(original).String(), Data: invalid}))

	result, err := MigrateLegacyEntries(t.Context(), client)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Skipped: 2}, result)
	require.NotNil(t, readTracked(t, client, other))
	require.NotNil(t, readTracked(t, client, LegacyIDFor(original)))
}

func Test_MigrateLegacyEntries_MultipleGroupsAndPlanes(t *testing.T) {
	t.Parallel()
	client := inmemory.NewClient()
	groups := []string{migrateGroup, "/planes/radius/other/resourceGroups/second"}
	ids := []resources.ID{}
	for _, group := range groups {
		require.NoError(t, client.Save(t.Context(), &database.Object{ID: group, Data: map[string]any{}}))
		id := resources.MustParse(group + "/providers/Applications.Test/testResources/r")
		saveTracked(t, client, id, LegacyIDFor(id), v1.ProvisioningStateSucceeded)
		ids = append(ids, id)
	}

	result, err := MigrateLegacyEntries(t.Context(), client)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Migrated: 2}, result)
	for _, id := range ids {
		require.NotNil(t, readTracked(t, client, IDFor(id)))
		require.Nil(t, readTracked(t, client, LegacyIDFor(id)))
	}
}

// racingClient updates the legacy entry after the migration copies it, as a concurrent proxy request would.
type racingClient struct {
	database.Client
	legacyID string
	raced    bool
}

func (c *racingClient) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if err := c.Client.Save(ctx, obj, options...); err != nil {
		return err
	}
	if c.raced || obj.ID == c.legacyID {
		return nil
	}

	c.raced = true
	legacy, err := c.Client.Get(ctx, c.legacyID)
	if err != nil {
		return err
	}
	entry := datamodel.GenericResource{}
	if err := legacy.As(&entry); err != nil {
		return err
	}
	entry.AsyncProvisioningState = v1.ProvisioningStateAccepted
	return c.Client.Save(ctx, &database.Object{ID: c.legacyID, Data: entry}, database.WithETag(legacy.ETag))
}

func Test_MigrateLegacyEntries_ConcurrentUpdate(t *testing.T) {
	t.Parallel()
	original := resources.MustParse(migrateGroup + "/providers/Applications.Test/testResources/my-resource")
	store := inmemory.NewClient()
	require.NoError(t, store.Save(t.Context(), &database.Object{ID: migrateGroup, Data: map[string]any{}}))
	saveTracked(t, store, original, LegacyIDFor(original), v1.ProvisioningStateSucceeded)
	client := &racingClient{Client: store, legacyID: LegacyIDFor(original).String()}

	result, err := MigrateLegacyEntries(t.Context(), client)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Remaining: 1}, result, "a concurrently updated legacy entry must not be deleted")
	require.Equal(t, v1.ProvisioningStateAccepted, readTracked(t, store, LegacyIDFor(original)).AsyncProvisioningState)
	require.NotNil(t, readTracked(t, store, IDFor(original)))

	// Once the queued operation finishes, the legacy entry is idle and the next pass removes the redundant copy.
	legacy, err := store.Get(t.Context(), LegacyIDFor(original).String())
	require.NoError(t, err)
	entry := datamodel.GenericResource{}
	require.NoError(t, legacy.As(&entry))
	entry.AsyncProvisioningState = v1.ProvisioningStateSucceeded
	require.NoError(t, store.Save(t.Context(), &database.Object{ID: legacy.ID, Data: entry}))

	result, err = MigrateLegacyEntries(t.Context(), store)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Deduplicated: 1}, result)
	require.Nil(t, readTracked(t, store, LegacyIDFor(original)))
}

// creatingClient creates the current entry right before the migration copies the legacy entry, as another writer would.
type creatingClient struct {
	database.Client
	currentID string
	existing  *datamodel.GenericResource
}

func (c *creatingClient) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if obj.ID == c.currentID && c.existing != nil {
		if err := c.Client.Save(ctx, &database.Object{ID: c.currentID, Data: c.existing}); err != nil {
			return err
		}
		c.existing = nil
	}
	return c.Client.Save(ctx, obj, options...)
}

func Test_MigrateLegacyEntries_CurrentCreatedConcurrently(t *testing.T) {
	t.Parallel()
	original := resources.MustParse(migrateGroup + "/providers/Applications.Test/testResources/my-resource")
	for storeName, newStore := range newStores(t) {
		t.Run(storeName, func(t *testing.T) {
			t.Parallel()
			store := newStore()
			require.NoError(t, store.Save(t.Context(), &database.Object{ID: migrateGroup, Data: map[string]any{"name": "test-group"}}))
			saveTracked(t, store, original, LegacyIDFor(original), v1.ProvisioningStateSucceeded)
			existing := datamodel.GenericResourceFromID(original, IDFor(original))
			existing.Properties.APIVersion = "2024-01-01"
			client := &creatingClient{Client: store, currentID: IDFor(original).String(), existing: existing}

			result, err := MigrateLegacyEntries(t.Context(), client)
			require.NoError(t, err)
			require.Equal(t, hashmigration.Result{Remaining: 1}, result)
			require.Equal(t, "2024-01-01", readTracked(t, store, IDFor(original)).Properties.APIVersion, "the concurrently created entry must not be overwritten")
			require.NotNil(t, readTracked(t, store, LegacyIDFor(original)))

			result, err = MigrateLegacyEntries(t.Context(), store)
			require.NoError(t, err)
			require.Equal(t, hashmigration.Result{Deduplicated: 1}, result)
			require.Nil(t, readTracked(t, store, LegacyIDFor(original)))
			require.Equal(t, "2024-01-01", readTracked(t, store, IDFor(original)).Properties.APIVersion)
		})
	}
}

type failingQueryClient struct {
	database.Client
	err error
}

func (c *failingQueryClient) Query(context.Context, database.Query, ...database.QueryOptions) (*database.ObjectQueryResult, error) {
	return nil, c.err
}

func Test_MigrateLegacyEntries_QueryError(t *testing.T) {
	t.Parallel()
	failure := errors.New("unavailable")
	_, err := MigrateLegacyEntries(t.Context(), &failingQueryClient{Client: inmemory.NewClient(), err: failure})
	require.ErrorIs(t, err, failure)
}
