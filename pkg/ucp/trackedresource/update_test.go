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

package trackedresource

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var (
	testURL = func() *url.URL {
		u, err := url.Parse("http://example.com/some-url")
		if err != nil {
			panic(err)
		}
		return u
	}()
)

func setupUpdater(t *testing.T) (*Updater, *database.MockClient, *mockRoundTripper) {
	ctrl := gomock.NewController(t)

	databaseClient := database.NewMockClient(ctrl)
	roundTripper := &mockRoundTripper{}
	updater := NewUpdater(databaseClient, &http.Client{Transport: roundTripper})

	// Optimize these values for testability. We don't want to wait for retries or timeouts unless
	// the test is specifically testing that behavior.
	updater.RetryDelay = time.Millisecond * 100
	updater.AttemptCount = 1
	updater.RequestTimeout = time.Microsecond * 100

	return updater, databaseClient, roundTripper
}

func Test_ResolveTrackingEntry(t *testing.T) {
	t.Parallel()
	currentID, legacyID := IDFor(testID), LegacyIDFor(testID)
	current := &database.Object{ID: currentID.String(), ETag: "current-etag"}
	legacy := &database.Object{ID: legacyID.String(), ETag: "legacy-etag"}
	notFound := &database.ErrNotFound{}
	failure := errors.New("database unavailable")
	for _, tt := range []struct {
		name       string
		current    *database.Object
		currentErr error
		legacy     *database.Object
		legacyErr  error

		// recheck and recheckErr are returned when the current ID is read again after both reads found nothing.
		recheck    *database.Object
		recheckErr error

		wantID  resources.ID
		want    *database.Object
		wantErr error
	}{
		{name: "current only", current: current, wantID: currentID, want: current},
		{name: "both present prefer current", current: current, legacy: legacy, wantID: currentID, want: current},
		{name: "legacy only", currentErr: notFound, legacy: legacy, wantID: legacyID, want: legacy},
		{name: "neither present", currentErr: notFound, legacyErr: notFound, recheckErr: notFound, wantID: currentID},
		{name: "moved between the reads", currentErr: notFound, legacyErr: notFound, recheck: current, wantID: currentID, want: current},
		{name: "current read error", currentErr: failure, wantID: currentID, wantErr: failure},
		{name: "legacy read error", currentErr: notFound, legacyErr: failure, wantID: currentID, wantErr: failure},
		{name: "recheck error", currentErr: notFound, legacyErr: notFound, recheckErr: failure, wantID: currentID, wantErr: failure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := database.NewMockClient(gomock.NewController(t))
			first := client.EXPECT().Get(gomock.Any(), currentID.String()).Return(tt.current, tt.currentErr)
			if errors.Is(tt.currentErr, notFound) {
				second := client.EXPECT().Get(gomock.Any(), legacyID.String()).Return(tt.legacy, tt.legacyErr).After(first.Call)
				if errors.Is(tt.legacyErr, notFound) {
					// The migration saves the current entry before it deletes the legacy entry, so a move between the
					// two reads is found by reading the current ID again.
					client.EXPECT().Get(gomock.Any(), currentID.String()).Return(tt.recheck, tt.recheckErr).After(second)
				}
			}
			id, obj, err := ResolveTrackingEntry(t.Context(), client, testID)
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.wantID, id)
			require.Same(t, tt.want, obj)
		})
	}
}

func Test_Update(t *testing.T) {
	t.Run("successful update", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)
		newEntry := true

		apiVersion := "1234"
		resource := map[string]any{
			"id":         testID.String(),
			"name":       testID.Name(),
			"type":       testID.Type(),
			"properties": map[string]any{},
		}

		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)
		databaseClient.EXPECT().
			Get(gomock.Any(), LegacyIDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
				require.Equal(t, IDFor(testID).String(), obj.ID)
				require.Equal(t, newEntry, database.NewSaveConfig(options...).CreateOnly, "a new entry must not overwrite one created concurrently")

				dm := obj.Data.(*datamodel.GenericResource)
				require.Equal(t, IDFor(testID).String(), dm.ID)
				require.Equal(t, testID.String(), dm.Properties.ID)
				require.Equal(t, apiVersion, dm.Properties.APIVersion)
				return nil
			}).
			Times(1)

		err := updater.Update(t.Context(), testURL.String(), testID, apiVersion)
		require.NoError(t, err)
	})

	t.Run("retry then success", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)
		newEntry := true
		updater.AttemptCount = 2

		apiVersion := "1234"
		resource := map[string]any{
			"id":         testID.String(),
			"name":       testID.Name(),
			"type":       testID.Type(),
			"properties": map[string]any{},
		}

		// Fail once, then succeed.
		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, errors.New("this will be retried")).
			Times(1)
		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)
		databaseClient.EXPECT().
			Get(gomock.Any(), LegacyIDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
				require.Equal(t, IDFor(testID).String(), obj.ID)
				require.Equal(t, newEntry, database.NewSaveConfig(options...).CreateOnly, "a new entry must not overwrite one created concurrently")

				dm := obj.Data.(*datamodel.GenericResource)
				require.Equal(t, IDFor(testID).String(), dm.ID)
				require.Equal(t, testID.String(), dm.Properties.ID)
				require.Equal(t, apiVersion, dm.Properties.APIVersion)
				return nil
			}).
			Times(1)

		err := updater.Update(t.Context(), testURL.String(), testID, apiVersion)
		require.NoError(t, err)
	})

	t.Run("resource still provisioning", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)

		apiVersion := "1234"
		resource := map[string]any{
			"id":   testID.String(),
			"name": testID.Name(),
			"type": testID.Type(),
			"properties": map[string]any{
				"provisioningState": v1.ProvisioningStateAccepted,
			},
		}

		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)
		databaseClient.EXPECT().
			Get(gomock.Any(), LegacyIDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)

		// Mock a successful (non-terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		err := updater.Update(t.Context(), testURL.String(), testID, apiVersion)
		require.Error(t, err)
		require.ErrorIs(t, err, &InProgressErr{})
	})

	t.Run("tracked resource updated concurrently", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)

		apiVersion := "1234"
		resource := map[string]any{
			"id":   testID.String(),
			"name": testID.Name(),
			"type": testID.Type(),
		}

		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)
		databaseClient.EXPECT().
			Get(gomock.Any(), LegacyIDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)

		// Fail the "Save" operation due to a concurrent update.
		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&database.ErrConcurrency{}).
			Times(1)

		// Mock a successful (non-terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		err := updater.Update(t.Context(), testURL.String(), testID, apiVersion)
		require.Error(t, err)
		require.ErrorIs(t, err, &InProgressErr{})
	})

	t.Run("retries exhausted", func(t *testing.T) {
		updater, databaseClient, _ := setupUpdater(t)
		updater.AttemptCount = 3

		apiVersion := "1234"

		// Fail enough times to exhaust our retries.
		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, errors.New("this will be retried")).
			Times(3)

		err := updater.Update(t.Context(), testURL.String(), testID, apiVersion)
		require.Error(t, err)
		require.Equal(t, "failed to update tracked resource after 3 attempts", err.Error())
	})
}

func Test_run(t *testing.T) {
	apiVersion := "1234"

	t.Run("successful update (new resource)", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)
		newEntry := true

		resource := map[string]any{
			"id":         testID.String(),
			"name":       testID.Name(),
			"type":       testID.Type(),
			"properties": map[string]any{},
		}

		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)
		databaseClient.EXPECT().
			Get(gomock.Any(), LegacyIDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
				require.Equal(t, IDFor(testID).String(), obj.ID)
				require.Equal(t, newEntry, database.NewSaveConfig(options...).CreateOnly, "a new entry must not overwrite one created concurrently")

				dm := obj.Data.(*datamodel.GenericResource)
				require.Equal(t, IDFor(testID).String(), dm.ID)
				require.Equal(t, testID.String(), dm.Properties.ID)
				require.Equal(t, apiVersion, dm.Properties.APIVersion)
				return nil
			}).
			Times(1)

		err := updater.run(t.Context(), testID, testURL, apiVersion)
		require.NoError(t, err)
	})

	t.Run("successful update (legacy entry)", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)

		resource := map[string]any{
			"id":         testID.String(),
			"name":       testID.Name(),
			"type":       testID.Type(),
			"properties": map[string]any{},
		}

		etag := "some-etag"
		dm := datamodel.GenericResourceFromID(testID, LegacyIDFor(testID))
		dm.Properties.APIVersion = apiVersion

		// No entry exists under the current (SHA-256) ID, but an entry written by an older
		// version of Radius exists under the legacy (SHA-1) ID. We must find and update it in
		// place rather than orphaning it under a new ID.
		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)
		databaseClient.EXPECT().
			Get(gomock.Any(), LegacyIDFor(testID).String()).
			Return(&database.Object{ETag: etag, Data: dm}, nil).
			Times(1)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
				require.Equal(t, LegacyIDFor(testID).String(), obj.ID)
				require.Equal(t, etag, database.NewSaveConfig(options...).ETag)

				dm := obj.Data.(*datamodel.GenericResource)
				require.Equal(t, LegacyIDFor(testID).String(), dm.ID)
				require.Equal(t, testID.String(), dm.Properties.ID)
				require.Equal(t, apiVersion, dm.Properties.APIVersion)
				return nil
			}).
			Times(1)

		err := updater.run(t.Context(), testID, testURL, apiVersion)
		require.NoError(t, err)
	})

	t.Run("successful update (existing resource)", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)
		newEntry := false

		resource := map[string]any{
			"id":         testID.String(),
			"name":       testID.Name(),
			"type":       testID.Type(),
			"properties": map[string]any{},
		}

		etag := "some-etag"
		dm := datamodel.GenericResourceFromID(testID, IDFor(testID))
		dm.Properties.APIVersion = apiVersion

		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(&database.Object{ETag: etag, Data: dm}, nil).
			Times(1)
		databaseClient.EXPECT().
			Get(gomock.Any(), LegacyIDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(1)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
				require.Equal(t, IDFor(testID).String(), obj.ID)
				require.Equal(t, newEntry, database.NewSaveConfig(options...).CreateOnly, "a new entry must not overwrite one created concurrently")

				dm := obj.Data.(*datamodel.GenericResource)
				require.Equal(t, IDFor(testID).String(), dm.ID)
				require.Equal(t, testID.String(), dm.Properties.ID)
				require.Equal(t, apiVersion, dm.Properties.APIVersion)
				return nil
			}).
			Times(1)

		err := updater.run(t.Context(), testID, testURL, apiVersion)
		require.NoError(t, err)
	})

	t.Run("successful delete", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			id   resources.ID

			// stale records that an interrupted migration left an entry under the other tracking ID.
			stale bool

			// staleErr is returned when the entry under the other tracking ID is deleted.
			staleErr error
		}{
			{name: "current", id: IDFor(testID)},
			{name: "legacy", id: LegacyIDFor(testID)},
			{name: "both tracking IDs", id: IDFor(testID), stale: true},
			{name: "other entry updated concurrently", id: IDFor(testID), stale: true, staleErr: &database.ErrConcurrency{}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				updater, databaseClient, roundTripper := setupUpdater(t)
				etag := "some-etag"
				dm := datamodel.GenericResourceFromID(testID, tt.id)
				dm.Properties.APIVersion = apiVersion
				other := LegacyIDFor(testID)
				if tt.id.String() == other.String() {
					other = IDFor(testID)
					databaseClient.EXPECT().Get(gomock.Any(), IDFor(testID).String()).Return(nil, &database.ErrNotFound{})
				}
				resolved := databaseClient.EXPECT().Get(gomock.Any(), tt.id.String()).Return(&database.Object{ETag: etag, Data: dm}, nil)

				// The entry under the other tracking ID is read before the resource is fetched.
				var stale *database.Object
				if tt.stale {
					stale = &database.Object{ID: other.String(), ETag: "stale-etag"}
				}
				read := databaseClient.EXPECT().Get(gomock.Any(), other.String()).DoAndReturn(func(context.Context, string, ...database.GetOptions) (*database.Object, error) {
					require.Zero(t, roundTripper.Requests.Load(), "the other entry must be read before the resource is fetched")
					if stale == nil {
						return nil, &database.ErrNotFound{}
					}
					return stale, nil
				}).After(resolved.Call)
				roundTripper.RespondWithJSON(t, http.StatusNotFound, &v1.ErrorResponse{Error: &v1.ErrorDetails{Code: v1.CodeNotFound}})
				deleted := databaseClient.EXPECT().Delete(gomock.Any(), tt.id.String(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, options ...database.DeleteOptions) error {
						require.Equal(t, etag, database.NewDeleteConfig(options...).ETag)
						return nil
					}).After(read)

				// The entry is deleted under the other tracking ID too, so it does not keep listing the deleted resource.
				if !tt.stale {
					require.NoError(t, updater.run(t.Context(), testID, testURL, apiVersion))
					return
				}

				databaseClient.EXPECT().Delete(gomock.Any(), other.String(), gomock.Any()).
					DoAndReturn(func(_ context.Context, _ string, options ...database.DeleteOptions) error {
						require.Equal(t, "stale-etag", database.NewDeleteConfig(options...).ETag)
						return tt.staleErr
					}).After(deleted)
				err := updater.run(t.Context(), testID, testURL, apiVersion)
				if tt.staleErr != nil {
					require.ErrorIs(t, err, tt.staleErr, "a concurrent update of the other entry is retried")
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("delete keeps entries created during the fetch", func(t *testing.T) {
		for _, tt := range []struct {
			name string

			// legacy records that the resource was tracked under its legacy ID before the fetch.
			legacy bool
		}{
			{name: "resource recreated after a legacy entry", legacy: true},
			{name: "resource created while untracked"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				updater, databaseClient, roundTripper := setupUpdater(t)
				legacyID := LegacyIDFor(testID)

				// A request recreates the resource and its entry under the current ID after the fetch. Every read must
				// happen before the fetch, and the mock fails the test if an entry that did not exist before the fetch
				// is deleted.
				notFoundBeforeFetch := func(context.Context, string, ...database.GetOptions) (*database.Object, error) {
					require.Zero(t, roundTripper.Requests.Load(), "entries must be read before the resource is fetched")
					return nil, &database.ErrNotFound{}
				}
				databaseClient.EXPECT().Get(gomock.Any(), IDFor(testID).String()).DoAndReturn(notFoundBeforeFetch).Times(2)
				if !tt.legacy {
					databaseClient.EXPECT().Get(gomock.Any(), legacyID.String()).DoAndReturn(notFoundBeforeFetch).Times(2)
				} else {
					dm := datamodel.GenericResourceFromID(testID, legacyID)
					databaseClient.EXPECT().Get(gomock.Any(), legacyID.String()).Return(&database.Object{ETag: "legacy-etag", Data: dm}, nil)
					databaseClient.EXPECT().Delete(gomock.Any(), legacyID.String(), gomock.Any()).
						DoAndReturn(func(_ context.Context, _ string, options ...database.DeleteOptions) error {
							require.Equal(t, "legacy-etag", database.NewDeleteConfig(options...).ETag)
							return nil
						})
				}

				roundTripper.RespondWithJSON(t, http.StatusNotFound, &v1.ErrorResponse{Error: &v1.ErrorDetails{Code: v1.CodeNotFound}})
				require.NoError(t, updater.run(t.Context(), testID, testURL, apiVersion))
				require.Equal(t, int32(1), roundTripper.Requests.Load())
			})
		}
	})

	t.Run("resource still provisioning", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)

		resource := map[string]any{
			"id":   testID.String(),
			"name": testID.Name(),
			"type": testID.Type(),
			"properties": map[string]any{
				"provisioningState": v1.ProvisioningStateAccepted,
			},
		}

		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)
		databaseClient.EXPECT().
			Get(gomock.Any(), LegacyIDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(2)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		err := updater.run(t.Context(), testID, testURL, apiVersion)
		require.Error(t, err)
		require.ErrorIs(t, err, &InProgressErr{})
	})
}

func Test_fetch(t *testing.T) {
	resource := map[string]any{
		"id":   testID.String(),
		"name": testID.Name(),
		"type": testID.Type(),
		"properties": map[string]any{
			"provisioningState": v1.ProvisioningStateAccepted,
		},
	}

	errorResponse := &v1.ErrorResponse{
		Error: &v1.ErrorDetails{
			Code:    "SomeErrorCode",
			Message: "This is a test.",
		},
	}
	b, err := json.MarshalIndent(errorResponse, "", "  ")
	require.NoError(t, err)
	errorResponseText := string(b)

	t.Run("successful fetch (200)", func(t *testing.T) {
		updater, _, roundTripper := setupUpdater(t)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		expected := &trackedResourceState{
			ID:   testID.String(),
			Name: testID.Name(),
			Type: testID.Type(),
			Properties: trackedResourceStateProperties{
				ProvisioningState: to.Ptr(v1.ProvisioningStateAccepted),
			},
		}

		state, err := updater.fetch(t.Context(), testURL)
		require.NoError(t, err)
		require.Equal(t, expected, state)
	})

	t.Run("successful fetch (404)", func(t *testing.T) {
		updater, _, roundTripper := setupUpdater(t)

		// We consider 404 a success case.
		roundTripper.RespondWithJSON(t, http.StatusNotFound, errorResponse)

		state, err := updater.fetch(t.Context(), testURL)
		require.NoError(t, err)
		require.Nil(t, state)
	})

	t.Run("failure (non-JSON)", func(t *testing.T) {
		updater, _, roundTripper := setupUpdater(t)

		w := httptest.NewRecorder()
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("LOL here's some not-JSON"))
		roundTripper.Response = w.Result()

		state, err := updater.fetch(t.Context(), testURL)
		require.Error(t, err)
		require.Equal(t, "response is not JSON. Content-Type: \"text/plain\"", err.Error())
		require.Nil(t, state)
	})

	t.Run("failure (non-JSON)", func(t *testing.T) {
		updater, _, roundTripper := setupUpdater(t)

		roundTripper.RespondWithJSON(t, http.StatusBadRequest, errorResponse)

		state, err := updater.fetch(t.Context(), testURL)
		require.Error(t, err)
		require.Equal(t, "request failed with status code 400 Bad Request:\n"+errorResponseText, err.Error())
		require.Nil(t, state)
	})
}

type mockRoundTripper struct {
	Response *http.Response
	Err      error

	// Requests counts the requests sent.
	Requests atomic.Int32
}

func (rt *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.Requests.Add(1)
	return rt.Response, rt.Err
}

func (rt *mockRoundTripper) RespondWithJSON(t *testing.T, statusCode int, body any) {
	t.Helper()

	b, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(b)

	rt.Response = w.Result()
}
