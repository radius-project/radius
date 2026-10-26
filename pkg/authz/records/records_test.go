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

package records

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const dev = "/planes/radius/local/resourceGroups/dev"

func TestRecordCovers(t *testing.T) {
	t.Parallel()
	r := &Record{Actions: []string{"Applications.Core/containers/write"}, Targets: []string{dev}}
	require.True(t, r.Covers("applications.core/containers/write", dev+"/providers/Applications.Core/containers/c"))
	require.False(t, r.Covers("Applications.Core/containers/delete", dev+"/providers/Applications.Core/containers/c"))
	require.False(t, r.Covers("Applications.Core/containers/write", "/planes/radius/local/resourceGroups/prod/providers/Applications.Core/containers/c"))
	require.False(t, r.Covers("Applications.Core/containers/write", "/planes/radius/local/resourceGroups/devx/providers/Applications.Core/containers/c"))

	all := &Record{Actions: []string{ActionAll}, Targets: []string{dev}}
	require.True(t, all.Covers("x/y/write", dev))
}

func TestRecordIsSubsetOf(t *testing.T) {
	t.Parallel()
	parent := &Record{Actions: []string{ActionAll}, Targets: []string{dev}}
	require.True(t, (&Record{Actions: []string{ActionAll}, Targets: []string{dev + "/providers/A.B/c/d"}}).IsSubsetOf(parent))
	require.False(t, (&Record{Actions: []string{ActionAll}, Targets: []string{"/planes/radius/local/resourceGroups/prod"}}).IsSubsetOf(parent))
	require.False(t, (&Record{Actions: []string{ActionAll}}).IsSubsetOf(parent))

	narrow := &Record{Actions: []string{"A.B/c/write"}, Targets: []string{dev}}
	require.False(t, (&Record{Actions: []string{ActionAll}, Targets: []string{dev}}).IsSubsetOf(narrow))
	require.True(t, (&Record{Actions: []string{"A.B/c/write"}, Targets: []string{dev}}).IsSubsetOf(narrow))
}

func TestInputHash(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", InputHash(nil))
	require.Equal(t, InputHash([]byte(`{"a":1,"b":2}`)), InputHash([]byte("{ \"b\": 2,\n \"a\": 1 }")))
	require.NotEqual(t, InputHash([]byte(`{"a":1}`)), InputHash([]byte(`{"a":2}`)))
	require.Equal(t, InputHash([]byte(`{"a":1.50}`)), InputHash([]byte(`{"a": 1.50}`)))
}

func TestActionFor(t *testing.T) {
	t.Parallel()
	require.Equal(t, "A.B/c/write", ActionFor(http.MethodPut, "A.B/c"))
	require.Equal(t, "A.B/c/write", ActionFor(http.MethodPatch, "A.B/c"))
	require.Equal(t, "A.B/c/delete", ActionFor(http.MethodDelete, "A.B/c"))
	require.Equal(t, "A.B/c/action", ActionFor(http.MethodPost, "A.B/c"))
	require.Equal(t, "", ActionFor(http.MethodGet, "A.B/c"))
}

func TestResourceGroupScope(t *testing.T) {
	t.Parallel()
	id := resources.MustParse(dev + "/providers/Microsoft.Resources/deployments/d")
	require.Equal(t, dev, ResourceGroupScope(id))
}

func TestStore(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s := NewStore(inmemory.NewClient())

	parent := s.NewRecord(Record{Subject: "alice", Submitter: "bicep-de", Actions: []string{ActionAll}, Targets: []string{dev}})
	_, err := s.SaveRecord(ctx, parent, "")
	require.NoError(t, err)
	child := s.NewRecord(Record{Subject: "alice", Submitter: "bicep-de", Actions: []string{ActionAll}, Targets: []string{dev}, ParentID: parent.ID})
	_, err = s.SaveRecord(ctx, child, "")
	require.NoError(t, err)

	active, err := s.ActiveRecords(ctx, "bicep-de")
	require.NoError(t, err)
	require.Len(t, active, 2)

	op := &Operation{RecordID: parent.ID, ResourceID: dev + "/providers/A.B/c/d", Action: "A.B/c/write", AssignedComponent: "dynamic-rp"}
	require.NoError(t, s.CreateOperation(ctx, op))

	claimed, err := s.Claim(ctx, op.ID, "dynamic-rp")
	require.NoError(t, err)
	require.Equal(t, OperationRunning, claimed.Status)
	_, err = s.Claim(ctx, op.ID, "other")
	require.ErrorIs(t, err, ErrClaimed)
	_, err = s.Claim(ctx, op.ID, "dynamic-rp")
	require.NoError(t, err)

	require.NoError(t, s.FinishOperation(ctx, op.ID, OperationSucceeded))
	got, _, err := s.GetOperation(ctx, op.ID)
	require.NoError(t, err)
	require.False(t, got.Open())

	require.NoError(t, s.CloseRecord(ctx, parent.ID, RecordClosed))
	active, err = s.ActiveRecords(ctx, "")
	require.NoError(t, err)
	require.Empty(t, active)

	expired := s.NewRecord(Record{Targets: []string{dev}, ExpiresAt: time.Now().Add(-time.Minute)})
	require.False(t, expired.Active(time.Now()))
}

func TestHeaderTransport(t *testing.T) {
	t.Parallel()
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Header }))
	defer srv.Close()

	ctx := WithSourceNamespace(WithIDs(t.Context(), IDs{RecordID: "r", OperationID: "o"}), "ns")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := (&http.Client{Transport: &HeaderTransport{}}).Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, "r", got.Get(HeaderRecord))
	require.Equal(t, "o", got.Get(HeaderOperation))
	require.Equal(t, "ns", got.Get(HeaderSourceNamespace))
}

func TestGetAcceptsBareNames(t *testing.T) {
	ctx := t.Context()
	store := NewStore(inmemory.NewClient())
	rec := store.NewRecord(Record{Submitter: "ucp", Actions: []string{"*"}, Targets: []string{"/planes/radius/local/resourceGroups/dev"}})
	rec.ID = RecordID("dev-rec")
	_, err := store.SaveRecord(ctx, rec, "")
	require.NoError(t, err)

	got, _, err := store.GetRecord(ctx, "dev-rec")
	require.NoError(t, err)
	require.Equal(t, rec.ID, got.ID)

	_, _, err = store.GetRecord(ctx, "not a valid id/")
	require.ErrorIs(t, err, &database.ErrNotFound{})
}
