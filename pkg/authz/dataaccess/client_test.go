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

package dataaccess

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz/records"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/ucp/authzcheck"
	"github.com/stretchr/testify/require"
)

const testID = "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/c"

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *inmemory.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	inner := inmemory.NewClient()
	return &Client{Client: inner, Endpoint: srv.URL, HTTP: srv.Client()}, inner
}

func TestSaveGoesToUCP(t *testing.T) {
	var got authzcheck.DataSaveRequest
	var gotOp string
	c, inner := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, authzcheck.DataSavePath, r.URL.Path)
		gotOp = r.Header.Get(records.HeaderOperation)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_ = json.NewEncoder(w).Encode(authzcheck.DataResponse{ETag: "etag-1"})
	})

	ctx := records.WithIDs(t.Context(), records.IDs{RecordID: "r1", OperationID: "op1"})
	obj := &database.Object{Metadata: database.Metadata{ID: testID}, Data: map[string]any{"a": "b"}}
	require.NoError(t, c.Save(ctx, obj, database.WithETag("old")))

	require.Equal(t, testID, got.ID)
	require.Equal(t, "old", got.ETag)
	require.Equal(t, "op1", gotOp)
	require.Equal(t, database.ETag("etag-1"), obj.ETag)

	// The inner client was not written.
	_, err := inner.Get(ctx, testID)
	require.ErrorIs(t, err, &database.ErrNotFound{ID: testID})
}

func TestAuthorizationRecordsGoDirect(t *testing.T) {
	c, inner := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("UCP should not be called")
	})
	id := "/planes/radius/local/providers/System.Authorization/executionRecords/r1"
	require.NoError(t, c.Save(t.Context(), &database.Object{Metadata: database.Metadata{ID: id}, Data: map[string]any{}}))
	_, err := inner.Get(t.Context(), id)
	require.NoError(t, err)
	require.NoError(t, c.Delete(t.Context(), id))
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		check  func(t *testing.T, err error)
	}{
		{http.StatusNotFound, func(t *testing.T, err error) { require.ErrorIs(t, err, &database.ErrNotFound{ID: testID}) }},
		{http.StatusPreconditionFailed, func(t *testing.T, err error) { require.ErrorIs(t, err, &database.ErrConcurrency{}) }},
		{http.StatusForbidden, func(t *testing.T, err error) {
			var e *Error
			require.True(t, errors.As(err, &e))
			require.Equal(t, v1.CodeOperationNotAssigned, e.Code)
		}},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(v1.ErrorResponse{Error: &v1.ErrorDetails{Code: v1.CodeOperationNotAssigned, Message: "no"}})
			})
			tc.check(t, c.Delete(t.Context(), testID))
		})
	}
}
