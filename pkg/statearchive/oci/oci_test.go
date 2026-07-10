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

package oci

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/graph/persistence"
	graphstore "github.com/radius-project/radius/pkg/graph/persistence/git"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2"
	ocistore "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
)

func TestOCIArchive_CommitRoundTrip(t *testing.T) {
	archive, target := newTestArchive(t)
	ctx := context.Background()

	session, err := archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(session.Path(), "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "nested", "state.txt"), []byte("saved state"), 0o644))
	require.NoError(t, session.Commit(ctx, "ignored message"))
	session.Close(ctx)

	session, err = archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(ctx) })

	data, err := os.ReadFile(filepath.Join(session.Path(), "nested", "state.txt"))
	require.NoError(t, err)
	require.Equal(t, []byte("saved state"), data)

	pushes := target.PushCount()
	require.NoError(t, session.Commit(ctx, "ignored message"))
	require.Equal(t, pushes, target.PushCount(), "unchanged state must not upload again")
}

func TestOCIArchive_UsesOCIStorageForGraphs(t *testing.T) {
	archive, _ := newTestArchive(t)
	store, err := graphstore.NewStore(graphstore.Options{Archive: archive})
	require.NoError(t, err)

	key := persistence.Key{Namespace: "main", Name: "app"}
	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			{
				Name: to.Ptr("frontend"),
				Type: to.Ptr("Applications.Core/containers"),
			},
		},
	}
	require.NoError(t, store.Save(context.Background(), key, graph, persistence.SaveOptions{}))

	got, err := store.Load(context.Background(), key)
	require.NoError(t, err)
	require.Len(t, got.Resources, 1)
	require.Equal(t, "frontend", *got.Resources[0].Name)
}

func TestOCIArchive_OpenReturnsTargetError(t *testing.T) {
	archive := NewOCIArchive(Options{Repository: "example.test/state"})
	archive.newTarget = func(context.Context) (oras.Target, error) {
		return nil, errors.New("registry unavailable")
	}

	_, err := archive.Open(context.Background(), "radius-state")
	require.ErrorContains(t, err, "registry unavailable")
}

func TestOCIArchive_CommitReturnsTargetError(t *testing.T) {
	archive := NewOCIArchive(Options{Repository: "example.test/state"})
	archive.newTarget = func(context.Context) (oras.Target, error) {
		return failedPushTarget{}, nil
	}

	session, err := archive.Open(context.Background(), "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(context.Background()) })
	require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "state.txt"), []byte("state"), 0o644))

	err = session.Commit(context.Background(), "ignored message")
	require.ErrorContains(t, err, "push failed")
}

func newTestArchive(t *testing.T) (*OCIArchive, *countingTarget) {
	t.Helper()

	store, err := ocistore.New(t.TempDir())
	require.NoError(t, err)
	target := &countingTarget{Target: store}
	archive := NewOCIArchive(Options{Repository: "test/radius-state"})
	archive.newTarget = func(context.Context) (oras.Target, error) {
		return target, nil
	}
	return archive, target
}

type countingTarget struct {
	oras.Target
	mu     sync.Mutex
	pushes int
}

func (t *countingTarget) Push(ctx context.Context, desc ocispec.Descriptor, content io.Reader) error {
	t.mu.Lock()
	t.pushes++
	t.mu.Unlock()
	return t.Target.Push(ctx, desc, content)
}

func (t *countingTarget) PushCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pushes
}

type failedPushTarget struct{}

func (failedPushTarget) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, errdef.ErrNotFound
}

func (failedPushTarget) Fetch(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
	return nil, errdef.ErrNotFound
}

func (failedPushTarget) Exists(context.Context, ocispec.Descriptor) (bool, error) {
	return false, nil
}

func (failedPushTarget) Push(context.Context, ocispec.Descriptor, io.Reader) error {
	return errors.New("push failed")
}

func (failedPushTarget) Tag(context.Context, ocispec.Descriptor, string) error {
	return nil
}
