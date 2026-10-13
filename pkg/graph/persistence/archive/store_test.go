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

package archive

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/graph/persistence"
	"github.com/radius-project/radius/pkg/statearchive"
	"github.com/radius-project/radius/pkg/to"
	"go.uber.org/mock/gomock"
)

func TestNewStore_DefaultsArchiveName(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{Archive: newTestArchive(t)})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, DefaultGraphArchive, s.archiveName)
}

func TestNewStore_HonorsBranch(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{Branch: "custom", Archive: newTestArchive(t)})
	require.NoError(t, err)
	assert.Equal(t, "custom", s.archiveName)
}

func TestNewStore_HonorsArchiveName(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{ArchiveName: "custom", Archive: newTestArchive(t)})
	require.NoError(t, err)
	assert.Equal(t, "custom", s.archiveName)
}

func TestNewStore_RejectsConflictingArchiveNames(t *testing.T) {
	t.Parallel()

	_, err := NewStore(Options{ArchiveName: "archive-name", Branch: "branch-name", Archive: newTestArchive(t)})
	require.ErrorContains(t, err, "conflicts with deprecated branch option")
}

func TestNewStore_AcceptsMatchingArchiveNames(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{ArchiveName: "shared-name", Branch: "shared-name", Archive: newTestArchive(t)})
	require.NoError(t, err)
	assert.Equal(t, "shared-name", s.archiveName)
}

func TestNewStore_RequiresArchive(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{})
	require.Nil(t, s)
	require.ErrorContains(t, err, "graph store requires an Archive")
}

func TestStore_ReturnsArchiveOpenErrors(t *testing.T) {
	t.Parallel()
	key := persistence.Key{Namespace: "main", Name: "app-graph"}
	for _, tc := range []struct {
		name string
		run  func(context.Context, *Store) error
	}{
		{name: "save", run: func(ctx context.Context, s *Store) error {
			return s.Save(ctx, key, &corerpv20250801preview.ApplicationGraphResponse{}, persistence.SaveOptions{})
		}},
		{name: "load", run: func(ctx context.Context, s *Store) error {
			_, err := s.Load(ctx, key)
			return err
		}},
		{name: "list", run: func(ctx context.Context, s *Store) error {
			_, err := s.List(ctx, "")
			return err
		}},
		{name: "delete", run: func(ctx context.Context, s *Store) error {
			return s.Delete(ctx, key)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			openErr := errors.New("registry unavailable")
			archive := statearchive.NewMockArchive(gomock.NewController(t))
			archive.EXPECT().Open(gomock.Any(), DefaultGraphArchive).Return(nil, openErr)
			s, err := NewStore(Options{Archive: archive})
			require.NoError(t, err)
			require.ErrorIs(t, tc.run(t.Context(), s), openErr)
		})
	}
}

func TestStore_CommitsMutationsAndClosesOnFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		save      bool
		message   string
		commitErr error
	}{
		{name: "save", save: true, message: "radius: update main/app-graph.json"},
		{name: "custom save message", save: true, message: "custom"},
		{name: "failed save", save: true, message: "radius: update main/app-graph.json", commitErr: errors.New("upload failed")},
		{name: "delete", message: "radius: delete main/app-graph.json"},
		{name: "failed delete", message: "radius: delete main/app-graph.json", commitErr: errors.New("upload failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "main", "app-graph.json")
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte("{}"), 0o644))
			graph := &corerpv20250801preview.ApplicationGraphResponse{
				Resources: []*corerpv20250801preview.ApplicationGraphResource{{Name: to.Ptr("frontend")}},
			}
			session := statearchive.NewMockSession(ctrl)
			session.EXPECT().Path().Return(dir)
			session.EXPECT().Commit(gomock.Any(), tc.message).DoAndReturn(func(context.Context, string) error {
				if tc.save {
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					want, err := json.MarshalIndent(graph, "", "  ")
					require.NoError(t, err)
					require.Equal(t, want, data)
				} else {
					_, err := os.Stat(path)
					require.ErrorIs(t, err, os.ErrNotExist)
				}
				return tc.commitErr
			})
			session.EXPECT().Close(gomock.Any())
			archive := statearchive.NewMockArchive(ctrl)
			archive.EXPECT().Open(gomock.Any(), "custom-archive").Return(session, nil)
			s, err := NewStore(Options{ArchiveName: "custom-archive", Archive: archive})
			require.NoError(t, err)
			key := persistence.Key{Namespace: "main", Name: "app-graph"}
			if tc.save {
				opts := persistence.SaveOptions{}
				if tc.name == "custom save message" {
					opts.Message = tc.message
				}
				err = s.Save(t.Context(), key, graph, opts)
			} else {
				err = s.Delete(t.Context(), key)
			}
			require.ErrorIs(t, err, tc.commitErr)
		})
	}
}

func TestKeyFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want persistence.Key
	}{
		{
			name: "namespaced",
			path: "main/app.json",
			want: persistence.Key{Namespace: "main", Name: "app"},
		},
		{
			name: "single segment",
			path: "lonely.json",
			want: persistence.Key{Name: "lonely"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, keyFromPath(tc.path))
		})
	}
}

func TestSave_RejectsNilPayload(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{Archive: newTestArchive(t)})
	require.NoError(t, err)

	err = s.Save(t.Context(), persistence.Key{Namespace: "ns", Name: "n"}, nil, persistence.SaveOptions{})
	require.Error(t, err)
}

func TestStore_SaveLoadDeleteRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s, err := NewStore(Options{Archive: newTestArchive(t)})
	require.NoError(t, err)

	key := persistence.Key{Namespace: "main", Name: "app"}
	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			{
				ID:                to.Ptr("resource-id"),
				Name:              to.Ptr("frontend"),
				Type:              to.Ptr("Applications.Core/containers"),
				ProvisioningState: to.Ptr("Succeeded"),
			},
		},
	}

	require.NoError(t, s.Save(ctx, key, graph, persistence.SaveOptions{Message: "test save"}))

	got, err := s.Load(ctx, key)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Resources, 1)
	assert.Equal(t, "frontend", *got.Resources[0].Name)
	assert.Equal(t, "Applications.Core/containers", *got.Resources[0].Type)

	require.NoError(t, s.Delete(ctx, key))

	_, err = s.Load(ctx, key)
	require.Error(t, err)
	assert.True(t, errors.Is(err, persistence.ErrNotFound), "expected ErrNotFound after delete, got %v", err)
}

func TestStore_LoadMissingKeyReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s, err := NewStore(Options{Archive: newTestArchive(t)})
	require.NoError(t, err)

	_, err = s.Load(ctx, persistence.Key{Namespace: "ns", Name: "missing"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, persistence.ErrNotFound))
}

func TestStore_DeleteMissingKeyReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{Archive: newTestArchive(t)})
	require.NoError(t, err)

	err = s.Delete(t.Context(), persistence.Key{Namespace: "ns", Name: "missing"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, persistence.ErrNotFound))
}

func TestStore_List(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	s, err := NewStore(Options{Archive: newTestArchive(t)})
	require.NoError(t, err)

	keys := []persistence.Key{
		{Namespace: "main", Name: "app"},
		{Namespace: "main", Name: "other"},
		{Namespace: "feature", Name: "other"},
	}
	for _, k := range keys {
		require.NoError(t, s.Save(ctx, k, &corerpv20250801preview.ApplicationGraphResponse{}, persistence.SaveOptions{}))
	}

	got, err := s.List(ctx, "main")
	require.NoError(t, err)

	// Sort for deterministic comparison.
	sort.Slice(got, func(i, j int) bool {
		return got[i].Name < got[j].Name
	})

	require.Len(t, got, 2)
	assert.Equal(t, persistence.Key{Namespace: "main", Name: "app"}, got[0])
	assert.Equal(t, persistence.Key{Namespace: "main", Name: "other"}, got[1])

	all, err := s.List(ctx, "")
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestStore_ListMissingNamespaceReturnsEmpty(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{Archive: newTestArchive(t)})
	require.NoError(t, err)

	got, err := s.List(t.Context(), "does-not-exist")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestStore_ListRejectsInvalidNamespace(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{Archive: newTestArchive(t)})
	require.NoError(t, err)

	tests := []struct {
		name      string
		namespace string
	}{
		{name: "dot-dot", namespace: ".."},
		{name: "dot", namespace: "."},
		{name: "forward slash", namespace: "a/b"},
		{name: "backslash", namespace: `a\b`},
		{name: "NUL", namespace: "a\x00b"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.List(t.Context(), tc.namespace)
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

func TestConstructPathForKey_RejectsEmptyNamespaceOrName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  persistence.Key
	}{
		{name: "empty namespace", key: persistence.Key{Name: "n"}},
		{name: "empty name", key: persistence.Key{Namespace: "ns"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := constructPathForKey(tc.key)
			require.Error(t, err)
		})
	}
}

func TestConstructPathForKey_RejectsTraversalAndSeparators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  persistence.Key
	}{
		{name: "namespace dot-dot", key: persistence.Key{Namespace: "..", Name: "n"}},
		{name: "name dot-dot", key: persistence.Key{Namespace: "ns", Name: ".."}},
		{name: "namespace dot", key: persistence.Key{Namespace: ".", Name: "n"}},
		{name: "name dot", key: persistence.Key{Namespace: "ns", Name: "."}},
		{name: "namespace forward slash", key: persistence.Key{Namespace: "a/b", Name: "n"}},
		{name: "name forward slash", key: persistence.Key{Namespace: "ns", Name: "a/b"}},
		{name: "namespace backslash", key: persistence.Key{Namespace: `a\b`, Name: "n"}},
		{name: "name backslash", key: persistence.Key{Namespace: "ns", Name: `a\b`}},
		{name: "namespace NUL", key: persistence.Key{Namespace: "a\x00b", Name: "n"}},
		{name: "name NUL", key: persistence.Key{Namespace: "ns", Name: "a\x00b"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := constructPathForKey(tc.key)
			require.Error(t, err)
		})
	}
}

func TestConstructPathForKey_AcceptsValidKey(t *testing.T) {
	t.Parallel()

	got, err := constructPathForKey(persistence.Key{Namespace: "main", Name: "app"})
	require.NoError(t, err)
	assert.Equal(t, "main/app.json", got)
}

// Compile-time assertion documenting that *Store satisfies persistence.Store
// (mirrors the runtime check in store.go and surfaces breakage in tests).
var _ persistence.Store = (*Store)(nil)
