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

package git

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/graph/persistence"
	"github.com/radius-project/radius/pkg/to"
)

func TestNewStore_DefaultsBranch(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, DefaultGraphBranch, s.branch)
}

func TestNewStore_HonorsBranch(t *testing.T) {
	t.Parallel()

	s, err := NewStore(Options{Branch: "custom"})
	require.NoError(t, err)
	assert.Equal(t, "custom", s.branch)
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

	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	s, err := NewStore(Options{Branch: "store-" + t.Name()})
	require.NoError(t, err)

	err = s.Save(context.Background(), persistence.Key{Namespace: "ns", Name: "n"}, nil, persistence.SaveOptions{})
	require.Error(t, err)
}

func TestStore_SaveLoadDeleteRoundTrip(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	s, err := NewStore(Options{Branch: "store-" + t.Name()})
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
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	s, err := NewStore(Options{Branch: "store-" + t.Name()})
	require.NoError(t, err)

	_, err = s.Load(ctx, persistence.Key{Namespace: "ns", Name: "missing"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, persistence.ErrNotFound))
}

func TestStore_DeleteMissingKeyReturnsErrNotFound(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	s, err := NewStore(Options{Branch: "store-" + t.Name()})
	require.NoError(t, err)

	err = s.Delete(context.Background(), persistence.Key{Namespace: "ns", Name: "missing"})
	_ = ctx
	require.Error(t, err)
	assert.True(t, errors.Is(err, persistence.ErrNotFound))
}

func TestStore_List(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	s, err := NewStore(Options{Branch: "store-" + t.Name()})
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
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	s, err := NewStore(Options{Branch: "store-" + t.Name()})
	require.NoError(t, err)

	got, err := s.List(context.Background(), "does-not-exist")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestStore_ListRejectsInvalidNamespace(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	s, err := NewStore(Options{Branch: "store-" + t.Name()})
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
			got, err := s.List(context.Background(), tc.namespace)
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
// (mirrors the runtime check in git_store.go and surfaces breakage in tests).
var _ persistence.Store = (*Store)(nil)
