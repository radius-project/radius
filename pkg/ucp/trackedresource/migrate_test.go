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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
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
	require.Nil(t, readTracked(t, store, IDFor(original)), "an idle copy of a busy entry must not stay where requests find it")

	// Once the queued operation finishes, the legacy entry is idle and the next pass moves it.
	legacy, err := store.Get(t.Context(), LegacyIDFor(original).String())
	require.NoError(t, err)
	entry := datamodel.GenericResource{}
	require.NoError(t, legacy.As(&entry))
	entry.AsyncProvisioningState = v1.ProvisioningStateSucceeded
	require.NoError(t, store.Save(t.Context(), &database.Object{ID: legacy.ID, Data: entry}))

	result, err = MigrateLegacyEntries(t.Context(), store)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Migrated: 1}, result)
	require.Nil(t, readTracked(t, store, LegacyIDFor(original)))
	require.NotNil(t, readTracked(t, store, IDFor(original)))
}

// usingClient changes the legacy entry and then the copy right after the migration saves the copy, as a request that
// updates the legacy entry and another request that finds the copy would.
type usingClient struct {
	database.Client
	currentID string
	legacyID  string
	done      bool
}

func (c *usingClient) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if err := c.Client.Save(ctx, obj, options...); err != nil {
		return err
	}
	if c.done || obj.ID != c.currentID {
		return nil
	}

	c.done = true
	for _, id := range []string{c.legacyID, c.currentID} {
		stored, err := c.Client.Get(ctx, id)
		if err != nil {
			return err
		}
		entry := datamodel.GenericResource{}
		if err := stored.As(&entry); err != nil {
			return err
		}
		entry.AsyncProvisioningState = v1.ProvisioningStateAccepted
		if err := c.Client.Save(ctx, &database.Object{ID: id, Data: entry}, database.WithETag(stored.ETag)); err != nil {
			return err
		}
	}
	return nil
}

func Test_MigrateLegacyEntries_CopyUsedBeforeRemoval(t *testing.T) {
	t.Parallel()
	original := resources.MustParse(migrateGroup + "/providers/Applications.Test/testResources/my-resource")
	for storeName, newStore := range newStores(t) {
		t.Run(storeName, func(t *testing.T) {
			t.Parallel()
			store := newStore()
			require.NoError(t, store.Save(t.Context(), &database.Object{ID: migrateGroup, Data: map[string]any{"name": "test-group"}}))
			saveTracked(t, store, original, LegacyIDFor(original), v1.ProvisioningStateSucceeded)
			client := &usingClient{Client: store, currentID: IDFor(original).String(), legacyID: LegacyIDFor(original).String()}

			// A copy that a request used is kept; both entries are busy and a later pass deduplicates them.
			result, err := MigrateLegacyEntries(t.Context(), client)
			require.NoError(t, err)
			require.Equal(t, hashmigration.Result{Remaining: 1}, result)
			require.True(t, client.done)
			require.Equal(t, v1.ProvisioningStateAccepted, readTracked(t, store, LegacyIDFor(original)).AsyncProvisioningState)
			require.Equal(t, v1.ProvisioningStateAccepted, readTracked(t, store, IDFor(original)).AsyncProvisioningState)
		})
	}
}

// deduplicatingClient runs another migrator right after the migration saves the copy, as a second UCP replica would.
type deduplicatingClient struct {
	database.Client
	currentID string
	result    hashmigration.Result
	done      bool
}

func (c *deduplicatingClient) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if err := c.Client.Save(ctx, obj, options...); err != nil {
		return err
	}
	if c.done || obj.ID != c.currentID {
		return nil
	}

	c.done = true
	result, err := MigrateLegacyEntries(ctx, c.Client)
	c.result = result
	return err
}

func Test_MigrateLegacyEntries_ConcurrentMigrators(t *testing.T) {
	t.Parallel()
	original := resources.MustParse(migrateGroup + "/providers/Applications.Test/testResources/my-resource")
	for storeName, newStore := range newStores(t) {
		t.Run(storeName, func(t *testing.T) {
			t.Parallel()
			store := newStore()
			require.NoError(t, store.Save(t.Context(), &database.Object{ID: migrateGroup, Data: map[string]any{"name": "test-group"}}))
			saveTracked(t, store, original, LegacyIDFor(original), v1.ProvisioningStateSucceeded)
			client := &deduplicatingClient{Client: store, currentID: IDFor(original).String()}

			// The other migrator deletes the legacy entry after the copy was saved. The first one must not mistake that
			// for a deleted resource and delete the copy, which would lose the entry.
			result, err := MigrateLegacyEntries(t.Context(), client)
			require.NoError(t, err)
			require.True(t, client.done)
			require.Equal(t, hashmigration.Result{Deduplicated: 1}, client.result)
			require.Equal(t, hashmigration.Result{}, result)
			require.Nil(t, readTracked(t, store, LegacyIDFor(original)))
			require.NotNil(t, readTracked(t, store, IDFor(original)), "the entry must not be lost")
		})
	}
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

// deletingClient deletes the legacy entry right after the migration saves the current entry, as the updater does when
// it resolved the legacy entry before the move and then found the resource gone.
type deletingClient struct {
	database.Client
	currentID string
	legacyID  string

	// recreate saves the current entry again after the delete, as when the resource is created again.
	recreate bool
	done     bool
}

func (c *deletingClient) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if err := c.Client.Save(ctx, obj, options...); err != nil {
		return err
	}
	if c.done || obj.ID != c.currentID {
		return nil
	}

	c.done = true
	if err := c.Client.Delete(ctx, c.legacyID); err != nil {
		return err
	}
	if !c.recreate {
		return nil
	}

	entry := datamodel.GenericResource{}
	if err := obj.As(&entry); err != nil {
		return err
	}
	entry.AsyncProvisioningState = v1.ProvisioningStateAccepted
	return c.Client.Save(ctx, &database.Object{ID: c.currentID, Data: entry})
}

func Test_MigrateLegacyEntries_DeletedDuringMove(t *testing.T) {
	t.Parallel()
	original := resources.MustParse(migrateGroup + "/providers/Applications.Test/testResources/my-resource")
	for storeName, newStore := range newStores(t) {
		for _, tt := range []struct {
			name     string
			recreate bool

			// currentRemains records that the entry under the current tracking ID must remain.
			currentRemains bool
		}{
			{name: "deleted"},
			{name: "deleted and created again", recreate: true, currentRemains: true},
		} {
			t.Run(storeName+"/"+tt.name, func(t *testing.T) {
				t.Parallel()
				store := newStore()
				require.NoError(t, store.Save(t.Context(), &database.Object{ID: migrateGroup, Data: map[string]any{"name": "test-group"}}))
				saveTracked(t, store, original, LegacyIDFor(original), v1.ProvisioningStateSucceeded)
				client := &deletingClient{Client: store, currentID: IDFor(original).String(), legacyID: LegacyIDFor(original).String(), recreate: tt.recreate}

				result, err := MigrateLegacyEntries(t.Context(), client)
				require.NoError(t, err)
				result.Message = ""
				require.Equal(t, hashmigration.Result{}, result)
				require.True(t, client.done)
				require.Nil(t, readTracked(t, store, LegacyIDFor(original)))
				if tt.currentRemains {
					require.NotNil(t, readTracked(t, store, IDFor(original)))
				} else {
					require.Nil(t, readTracked(t, store, IDFor(original)), "the move must not bring back the entry of a deleted resource")
				}
			})
		}
	}
}

type failingQueryClient struct {
	database.Client
	err error
}

func (c *failingQueryClient) Query(context.Context, database.Query, ...database.QueryOptions) (*database.ObjectQueryResult, error) {
	return nil, c.err
}

// notFoundTransport answers every request with a 404 after it runs during, so a test can act while the updater fetches
// a resource that is gone.
type notFoundTransport struct {
	during func()
}

func (rt *notFoundTransport) RoundTrip(*http.Request) (*http.Response, error) {
	rt.during()
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("{}"))
	return w.Result(), nil
}

// Two migrators read a legacy entry while it is idle, then a request queues an operation on it, and the updater finds
// the resource gone. While the updater fetches the resource, one migrator saves its copy of the entry and the other
// changes the salt of that copy, so the first cannot delete its copy once the updater deleted the legacy entry. The
// updater deletes the copy itself, so the gone resource is not listed.
func Test_MigrateLegacyEntries_UpdaterAndConcurrentMigrators(t *testing.T) {
	t.Parallel()
	original := resources.MustParse(migrateGroup + "/providers/Applications.Test/testResources/my-resource")
	legacyID, currentID := LegacyIDFor(original), IDFor(original)
	for name, newStore := range newStores(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := newStore()
			idle := saveTracked(t, client, original, legacyID, v1.ProvisioningStateSucceeded)
			entry := datamodel.GenericResource{}
			require.NoError(t, idle.As(&entry))
			saveTracked(t, client, original, legacyID, v1.ProvisioningStateAccepted)

			var copied *database.Object
			transport := &notFoundTransport{during: func() {
				moved := entry
				moved.ID = currentID.String()
				moved.Name = currentID.Name()
				copied = &database.Object{ID: currentID.String(), Data: &moved}
				require.NoError(t, client.Save(t.Context(), copied, database.WithCreateOnly()))
				current, err := client.Get(t.Context(), currentID.String())
				require.NoError(t, err)
				require.NoError(t, changeSalt(t.Context(), client, current))
			}}
			require.NoError(t, NewUpdater(client, &http.Client{Transport: transport}).run(t.Context(), original, testURL, "2023-01-01"))
			require.NotNil(t, copied)

			// The first migrator resumes: its delete of the legacy entry fails, and the entry is gone, so it deletes its copy
			// unless the copy changed.
			require.Error(t, client.Delete(t.Context(), legacyID.String(), database.WithETag(idle.ETag)))
			require.Equal(t, hashmigration.Result{}, removeUnusedCopy(t.Context(), client, logr.Discard(), legacyID.String(), copied))

			require.Nil(t, readTracked(t, client, currentID), "a gone resource must not be listed")
			require.Nil(t, readTracked(t, client, legacyID))
		})
	}
}

func Test_MigrateLegacyEntries_QueryError(t *testing.T) {
	t.Parallel()
	failure := errors.New("unavailable")
	_, err := MigrateLegacyEntries(t.Context(), &failingQueryClient{Client: inmemory.NewClient(), err: failure})
	require.ErrorIs(t, err, failure)
}
