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
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
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
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	ocistore "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
)

func TestOCIArchive_CommitRoundTrip(t *testing.T) {
	archive, target := newTestArchive(t)
	ctx := t.Context()

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

	require.NoError(t, store.Delete(context.Background(), key))
	_, err = store.Load(context.Background(), key)
	require.ErrorIs(t, err, persistence.ErrNotFound)
}

func TestOCIArchive_OpenRejectsEmptyName(t *testing.T) {
	archive, _ := newTestArchive(t)

	_, err := archive.Open(context.Background(), "")
	require.ErrorContains(t, err, "OCI archive name must not be empty")
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

func TestOCIArchive_CommitEnforcesGHCRVisibility(t *testing.T) {
	for _, test := range []struct {
		name          string
		visibility    packageVisibility
		errorContains string
	}{
		{name: "private", visibility: packageVisibilityPrivate},
		{name: "internal", visibility: packageVisibilityInternal},
		{name: "public", visibility: packageVisibilityPublic, errorContains: "is public"},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive, target := newTestArchive(t)
			checks := 0
			archive.checkPackageVisibility = func(context.Context) (packageVisibility, error) {
				checks++
				return test.visibility, nil
			}

			ctx := t.Context()
			session, err := archive.Open(ctx, "radius-state")
			require.NoError(t, err)
			t.Cleanup(func() { session.Close(ctx) })
			require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "state.txt"), []byte("state"), 0o644))

			err = session.Commit(ctx, "ignored message")
			if test.errorContains != "" {
				require.ErrorContains(t, err, test.errorContains)
				require.Equal(t, 0, target.PushCount(), "public packages must reject state before any upload")
			} else {
				require.NoError(t, err)
				require.Greater(t, target.PushCount(), 0)

				checksAfterUpload := checks
				require.NoError(t, session.Commit(ctx, "ignored message"))
				require.Equal(t, checksAfterUpload, checks, "unchanged state must not repeat the visibility check")
			}
			require.Equal(t, 1, checks)
		})
	}
}

func TestOCIArchive_CommitBootstrapsMissingGHCRPackage(t *testing.T) {
	for _, test := range []struct {
		name          string
		finalResult   visibilityResult
		errorContains string
	}{
		{
			name:        "private",
			finalResult: visibilityResult{visibility: packageVisibilityPrivate},
		},
		{
			name:        "internal",
			finalResult: visibilityResult{visibility: packageVisibilityInternal},
		},
		{
			name:          "public",
			finalResult:   visibilityResult{visibility: packageVisibilityPublic},
			errorContains: "is public",
		},
		{
			name:          "still not found",
			finalResult:   visibilityResult{err: errGHCRPackageNotFound},
			errorContains: "after empty archive bootstrap",
		},
		{
			name:          "API failure",
			finalResult:   visibilityResult{err: errors.New("API unavailable")},
			errorContains: "API unavailable",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive, target := newTestArchive(t)
			visibility := &visibilitySequence{results: []visibilityResult{
				{err: errGHCRPackageNotFound},
				test.finalResult,
			}}
			archive.checkPackageVisibility = visibility.Check

			ctx := t.Context()
			session, err := archive.Open(ctx, "radius-state")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "state.txt"), []byte("sensitive state"), 0o644))

			err = session.Commit(ctx, "ignored message")
			if test.errorContains == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.errorContains)
			}
			require.Equal(t, 2, visibility.calls)
			require.Greater(t, target.PushCount(), 0, "a missing package must be bootstrapped with an empty archive")
			session.Close(ctx)

			session, err = archive.Open(ctx, "radius-state")
			require.NoError(t, err)
			t.Cleanup(func() { session.Close(ctx) })
			if test.errorContains == "" {
				data, err := os.ReadFile(filepath.Join(session.Path(), "state.txt"))
				require.NoError(t, err)
				require.Equal(t, []byte("sensitive state"), data)
			} else {
				entries, err := os.ReadDir(session.Path())
				require.NoError(t, err)
				require.Empty(t, entries, "failed visibility verification must leave only the empty bootstrap archive")
			}
		})
	}
}

func TestOCIArchive_BootstrapDoesNotOverwriteConcurrentState(t *testing.T) {
	archive, target := newTestArchive(t)
	ctx := t.Context()
	checks := 0
	archive.checkPackageVisibility = func(context.Context) (packageVisibility, error) {
		checks++
		if checks == 1 {
			return "", errGHCRPackageNotFound
		}

		externalPath := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(externalPath, "state.txt"), []byte("external state"), 0o644))
		externalArtifact, err := createArtifact(ctx, "radius-state", externalPath)
		require.NoError(t, err)
		defer externalArtifact.Close(ctx)
		_, err = oras.Copy(ctx, externalArtifact.source, "radius-state", target, "radius-state", oras.DefaultCopyOptions)
		require.NoError(t, err)
		return packageVisibilityPrivate, nil
	}

	session, err := archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "state.txt"), []byte("local state"), 0o644))

	err = session.Commit(ctx, "ignored message")
	require.ErrorContains(t, err, "changed while this session was open")
	session.Close(ctx)

	session, err = archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(ctx) })
	data, err := os.ReadFile(filepath.Join(session.Path(), "state.txt"))
	require.NoError(t, err)
	require.Equal(t, []byte("external state"), data)
}

func TestOCIArchive_PublicPackageAllowsStateDeletion(t *testing.T) {
	archive, target := newTestArchive(t)
	ctx := t.Context()
	archive.checkPackageVisibility = func(context.Context) (packageVisibility, error) {
		return packageVisibilityPrivate, nil
	}

	session, err := archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "state.txt"), []byte("state"), 0o644))
	require.NoError(t, session.Commit(ctx, "ignored message"))
	session.Close(ctx)

	archive.checkPackageVisibility = func(context.Context) (packageVisibility, error) {
		return packageVisibilityPublic, nil
	}
	session, err = archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(session.Path(), "state.txt")))
	pushesBeforeDelete := target.PushCount()
	require.NoError(t, session.Commit(ctx, "ignored message"))
	require.Greater(t, target.PushCount(), pushesBeforeDelete)
	session.Close(ctx)

	session, err = archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(ctx) })
	entries, err := os.ReadDir(session.Path())
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestOCIArchive_CommitReturnsVisibilityErrorBeforeUpload(t *testing.T) {
	archive, target := newTestArchive(t)
	archive.checkPackageVisibility = func(context.Context) (packageVisibility, error) {
		return "", errors.New("visibility unavailable")
	}

	ctx := t.Context()
	session, err := archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(ctx) })
	require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "state.txt"), []byte("state"), 0o644))

	err = session.Commit(ctx, "ignored message")
	require.ErrorContains(t, err, "visibility unavailable")
	require.Equal(t, 0, target.PushCount())
}

func TestOCIArchive_CommitReturnsBootstrapUploadError(t *testing.T) {
	archive := NewOCIArchive(Options{Repository: "ghcr.io/radius-project/state"})
	archive.newTarget = func(context.Context) (oras.Target, error) {
		return failedPushTarget{}, nil
	}
	archive.checkPackageVisibility = func(context.Context) (packageVisibility, error) {
		return "", errGHCRPackageNotFound
	}

	ctx := t.Context()
	session, err := archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(ctx) })
	require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "state.txt"), []byte("state"), 0o644))

	err = session.Commit(ctx, "ignored message")
	require.ErrorContains(t, err, "failed to bootstrap GHCR package")
	require.ErrorContains(t, err, "push failed")
}

func TestOCIArchive_EmptyNewArchiveSkipsVisibilityCheck(t *testing.T) {
	archive, target := newTestArchive(t)
	checks := 0
	archive.checkPackageVisibility = func(context.Context) (packageVisibility, error) {
		checks++
		return packageVisibilityPublic, nil
	}

	ctx := t.Context()
	session, err := archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(ctx) })

	require.NoError(t, session.Commit(ctx, "ignored message"))
	require.Equal(t, 0, checks)
	require.Equal(t, 0, target.PushCount())
}

func TestOCIArchive_CommitPersistsDeletion(t *testing.T) {
	archive, target := newTestArchive(t)
	ctx := t.Context()

	session, err := archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "state.txt"), []byte("saved state"), 0o644))
	require.NoError(t, session.Commit(ctx, "ignored message"))
	session.Close(ctx)

	session, err = archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(session.Path(), "state.txt")))
	pushesBeforeDelete := target.PushCount()
	require.NoError(t, session.Commit(ctx, "ignored message"))
	require.Greater(t, target.PushCount(), pushesBeforeDelete, "emptying the archive must be persisted")
	session.Close(ctx)

	session, err = archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(ctx) })

	entries, err := os.ReadDir(session.Path())
	require.NoError(t, err)
	require.Empty(t, entries, "deletion must survive a reopen")
}

func TestOCIArchive_EmptyNewArchiveIsNoOp(t *testing.T) {
	archive, target := newTestArchive(t)
	ctx := t.Context()

	session, err := archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	require.NoError(t, session.Commit(ctx, "ignored message"))
	session.Close(ctx)

	require.Equal(t, 0, target.PushCount(), "an empty new archive must not upload anything")

	session, err = archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(ctx) })
	entries, err := os.ReadDir(session.Path())
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestOCIArchive_CommitRejectsConcurrentTagUpdate(t *testing.T) {
	archive, target := newTestArchive(t)
	ctx := t.Context()

	session, err := archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(session.Path(), "state.txt"), []byte("local state"), 0o644))

	externalPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(externalPath, "state.txt"), []byte("external state"), 0o644))
	externalArtifact, err := createArtifact(ctx, "radius-state", externalPath)
	require.NoError(t, err)
	defer externalArtifact.Close(ctx)
	_, err = oras.Copy(ctx, externalArtifact.source, "radius-state", target, "radius-state", oras.DefaultCopyOptions)
	require.NoError(t, err)

	err = session.Commit(ctx, "ignored message")
	require.ErrorContains(t, err, "changed while this session was open")
	session.Close(ctx)

	session, err = archive.Open(ctx, "radius-state")
	require.NoError(t, err)
	t.Cleanup(func() { session.Close(ctx) })
	data, err := os.ReadFile(filepath.Join(session.Path(), "state.txt"))
	require.NoError(t, err)
	require.Equal(t, []byte("external state"), data)
}

func TestOCIArchive_OpenRemoteTargetConfiguresPlainHTTP(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	archive := NewOCIArchive(Options{Repository: "localhost:5000/radius-state", PlainHTTP: true})

	target, err := archive.openRemoteTarget(context.Background())
	require.NoError(t, err)
	repository, ok := target.(*remote.Repository)
	require.True(t, ok)
	require.True(t, repository.PlainHTTP)
	require.NotNil(t, repository.Client)
}

func TestCreateLayerRejectsSymbolicLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("state"), 0o644))
	if err := os.Symlink(target, filepath.Join(root, "state-link")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	_, err := createLayer(root, io.Discard)
	require.ErrorContains(t, err, "unsupported symbolic link")
}

func TestCreateArtifactUsesCompatibleManifestAndCleansUp(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "state.txt"), []byte("state"), 0o644))

	var expectedLayer bytes.Buffer
	_, err := createLayer(root, &expectedLayer)
	require.NoError(t, err)

	artifact, err := createArtifact(ctx, "radius-state", root)
	require.NoError(t, err)
	require.DirExists(t, artifact.tempDir)

	manifestReader, err := artifact.source.Fetch(ctx, artifact.manifestDesc)
	require.NoError(t, err)
	manifestBytes, err := io.ReadAll(manifestReader)
	require.NoError(t, err)
	require.NoError(t, manifestReader.Close())

	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	require.Len(t, manifest.Layers, 1)
	require.Equal(t, layerMediaType, manifest.Layers[0].MediaType)
	require.Empty(t, manifest.Layers[0].Annotations, "file-store annotations must not change the archive manifest")

	expectedLayerDesc := content.NewDescriptorFromBytes(layerMediaType, expectedLayer.Bytes())
	expectedConfigDesc := content.NewDescriptorFromBytes(configMediaType, []byte("{}"))
	expectedManifest := ocispec.Manifest{
		Versioned:    manifest.Versioned,
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: configMediaType,
		Config:       expectedConfigDesc,
		Layers:       []ocispec.Descriptor{expectedLayerDesc},
	}
	expectedManifestBytes, err := json.Marshal(expectedManifest)
	require.NoError(t, err)
	expectedManifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, expectedManifestBytes)
	require.Equal(t, expectedManifestDesc, artifact.manifestDesc, "file-backed artifact must retain the previous manifest digest")

	tempDir := artifact.tempDir
	artifact.Close(ctx)
	require.NoDirExists(t, tempDir)
}

func TestCreateLayerPropagatesWriterErrors(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "state.txt"), []byte("state"), 0o644))

	_, err := createLayer(root, failingWriter{})
	require.ErrorContains(t, err, "write failed")
}

func TestArchivePathRejectsUnsafeNames(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "current directory", path: "."},
		{name: "parent directory", path: ".."},
		{name: "parent directory file", path: "../state.txt"},
		{name: "absolute", path: filepath.Join(t.TempDir(), "state.txt")},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := archivePath(t.TempDir(), test.path)
			require.ErrorContains(t, err, "invalid archive path")
		})
	}
}

func TestUnpackArchiveEntriesRejectsNonRegularEntries(t *testing.T) {
	for _, entry := range []tar.Header{
		{Name: "directory/", Typeflag: tar.TypeDir},
		{Name: "state-link", Typeflag: tar.TypeSymlink, Linkname: "state.txt"},
	} {
		t.Run(entry.Name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := tar.NewWriter(&archive)
			require.NoError(t, writer.WriteHeader(&entry))
			require.NoError(t, writer.Close())

			err := unpackArchiveEntries(&archive, t.TempDir())
			require.ErrorContains(t, err, "unsupported archive entry")
		})
	}
}

func TestUnpackArchiveRejectsInvalidArtifacts(t *testing.T) {
	ctx := t.Context()

	t.Run("malformed manifest", func(t *testing.T) {
		target := memory.New()
		data := []byte("{")
		manifestDesc := content.NewDescriptorFromBytes("application/octet-stream", data)
		require.NoError(t, target.Push(ctx, manifestDesc, bytes.NewReader(data)))

		err := unpackArchive(ctx, target, manifestDesc, t.TempDir())
		require.ErrorContains(t, err, "invalid OCI manifest")
	})

	t.Run("invalid layers", func(t *testing.T) {
		target := memory.New()
		manifestDesc := pushTestManifest(t, target, ocispec.Manifest{})

		err := unpackArchive(ctx, target, manifestDesc, t.TempDir())
		require.ErrorContains(t, err, "exactly one state archive layer")
	})

	t.Run("invalid compression", func(t *testing.T) {
		target := memory.New()
		layerDesc, err := pushBlob(ctx, target, layerMediaType, []byte("not gzip"))
		require.NoError(t, err)
		manifestDesc := pushTestManifest(t, target, ocispec.Manifest{Layers: []ocispec.Descriptor{layerDesc}})

		err = unpackArchive(ctx, target, manifestDesc, t.TempDir())
		require.ErrorContains(t, err, "invalid archive compression")
	})
}

func pushTestManifest(t *testing.T, target oras.Target, manifest ocispec.Manifest) ocispec.Descriptor {
	t.Helper()

	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	desc, err := pushBlob(context.Background(), target, ocispec.MediaTypeImageManifest, data)
	require.NoError(t, err)
	return desc
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

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type visibilityResult struct {
	visibility packageVisibility
	err        error
}

type visibilitySequence struct {
	results []visibilityResult
	calls   int
}

func (s *visibilitySequence) Check(context.Context) (packageVisibility, error) {
	result := s.results[s.calls]
	s.calls++
	return result.visibility, result.err
}
