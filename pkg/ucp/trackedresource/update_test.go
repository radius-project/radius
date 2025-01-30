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
	"testing"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/test/testcontext"
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

func Test_Update(t *testing.T) {
	t.Run("successful update", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)

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
			Times(1)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
				require.Equal(t, IDFor(testID).String(), obj.ID)

				dm := obj.Data.(*datamodel.GenericResource)
				require.Equal(t, IDFor(testID).String(), dm.ID)
				require.Equal(t, testID.String(), dm.Properties.ID)
				require.Equal(t, apiVersion, dm.Properties.APIVersion)
				return nil
			}).
			Times(1)

		err := updater.Update(testcontext.New(t), testURL.String(), testID, apiVersion)
		require.NoError(t, err)
	})

	t.Run("retry then success", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)
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
			Times(1)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
				require.Equal(t, IDFor(testID).String(), obj.ID)

				dm := obj.Data.(*datamodel.GenericResource)
				require.Equal(t, IDFor(testID).String(), dm.ID)
				require.Equal(t, testID.String(), dm.Properties.ID)
				require.Equal(t, apiVersion, dm.Properties.APIVersion)
				return nil
			}).
			Times(1)

		err := updater.Update(testcontext.New(t), testURL.String(), testID, apiVersion)
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
			Times(1)

		// Mock a successful (non-terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		err := updater.Update(testcontext.New(t), testURL.String(), testID, apiVersion)
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
			Times(1)

		// Fail the "Save" operation due to a concurrent update.
		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&database.ErrConcurrency{}).
			Times(1)

		// Mock a successful (non-terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		err := updater.Update(testcontext.New(t), testURL.String(), testID, apiVersion)
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

		err := updater.Update(testcontext.New(t), testURL.String(), testID, apiVersion)
		require.Error(t, err)
		require.Equal(t, "failed to update tracked resource after 3 attempts", err.Error())
	})
}

func Test_run(t *testing.T) {
	apiVersion := "1234"

	t.Run("successful update (new resource)", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)

		resource := map[string]any{
			"id":         testID.String(),
			"name":       testID.Name(),
			"type":       testID.Type(),
			"properties": map[string]any{},
		}

		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(nil, &database.ErrNotFound{}).
			Times(1)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
				require.Equal(t, IDFor(testID).String(), obj.ID)

				dm := obj.Data.(*datamodel.GenericResource)
				require.Equal(t, IDFor(testID).String(), dm.ID)
				require.Equal(t, testID.String(), dm.Properties.ID)
				require.Equal(t, apiVersion, dm.Properties.APIVersion)
				return nil
			}).
			Times(1)

		err := updater.run(testcontext.New(t), testID, IDFor(testID), testURL, apiVersion)
		require.NoError(t, err)
	})

	t.Run("successful update (existing resource)", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)

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
			Return(&database.Object{Metadata: database.Metadata{ETag: etag}, Data: dm}, nil).
			Times(1)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
				require.Equal(t, IDFor(testID).String(), obj.ID)

				dm := obj.Data.(*datamodel.GenericResource)
				require.Equal(t, IDFor(testID).String(), dm.ID)
				require.Equal(t, testID.String(), dm.Properties.ID)
				require.Equal(t, apiVersion, dm.Properties.APIVersion)
				return nil
			}).
			Times(1)

		err := updater.run(testcontext.New(t), testID, IDFor(testID), testURL, apiVersion)
		require.NoError(t, err)
	})

	t.Run("successful delete", func(t *testing.T) {
		updater, databaseClient, roundTripper := setupUpdater(t)

		etag := "some-etag"
		dm := datamodel.GenericResourceFromID(testID, IDFor(testID))
		dm.Properties.APIVersion = apiVersion

		databaseClient.EXPECT().
			Get(gomock.Any(), IDFor(testID).String()).
			Return(&database.Object{Metadata: database.Metadata{ETag: etag}, Data: dm}, nil).
			Times(1)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusNotFound, &v1.ErrorResponse{Error: &v1.ErrorDetails{Code: v1.CodeNotFound}})

		databaseClient.EXPECT().
			Delete(gomock.Any(), IDFor(testID).String(), gomock.Any()).
			Return(nil).
			Times(1)

		err := updater.run(testcontext.New(t), testID, IDFor(testID), testURL, apiVersion)
		require.NoError(t, err)
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
			Times(1)

		// Mock a successful (terminal) response from the downstream API.
		roundTripper.RespondWithJSON(t, http.StatusOK, resource)

		err := updater.run(testcontext.New(t), testID, IDFor(testID), testURL, apiVersion)
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

		state, err := updater.fetch(testcontext.New(t), testURL)
		require.NoError(t, err)
		require.Equal(t, expected, state)
	})

	t.Run("successful fetch (404)", func(t *testing.T) {
		updater, _, roundTripper := setupUpdater(t)

		// We consider 404 a success case.
		roundTripper.RespondWithJSON(t, http.StatusNotFound, errorResponse)

		state, err := updater.fetch(testcontext.New(t), testURL)
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

		state, err := updater.fetch(testcontext.New(t), testURL)
		require.Error(t, err)
		require.Equal(t, "response is not JSON. Content-Type: \"text/plain\"", err.Error())
		require.Nil(t, state)
	})

	t.Run("failure (non-JSON)", func(t *testing.T) {
		updater, _, roundTripper := setupUpdater(t)

		roundTripper.RespondWithJSON(t, http.StatusBadRequest, errorResponse)

		state, err := updater.fetch(testcontext.New(t), testURL)
		require.Error(t, err)
		require.Equal(t, "request failed with status code 400 Bad Request:\n"+errorResponseText, err.Error())
		require.Nil(t, state)
	})
}

type mockRoundTripper struct {
	Response *http.Response
	Err      error
}

func (rt *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
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
