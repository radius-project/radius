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

// Package oci implements the statearchive.Archive interface with OCI artifacts.
package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	credentials "oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"

	"github.com/radius-project/radius/pkg/statearchive"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	layerMediaType  = "application/vnd.radius.statearchive.layer.v1.tar+gzip"
	configMediaType = "application/vnd.radius.statearchive.config.v1+json"
)

var archiveLocks sync.Map // repository and archive name -> *sync.Mutex

// Options configures an OCIArchive.
type Options struct {
	// Repository is the OCI repository without a tag, for example
	// "ghcr.io/radius-project/radius-state".
	Repository string

	// PlainHTTP uses HTTP instead of HTTPS. It is intended for local test registries.
	PlainHTTP bool
}

type targetFactory func(context.Context) (oras.Target, error)

// OCIArchive is a statearchive.Archive backed by OCI artifacts.
type OCIArchive struct {
	options   Options
	newTarget targetFactory
}

// NewOCIArchive returns an OCI-backed state archive.
func NewOCIArchive(options Options) *OCIArchive {
	archive := &OCIArchive{options: options}
	archive.newTarget = archive.openRemoteTarget
	return archive
}

// Open materializes the archive tagged name in a temporary directory.
func (a *OCIArchive) Open(ctx context.Context, name string) (statearchive.Session, error) {
	if name == "" {
		return nil, errors.New("OCI archive name must not be empty")
	}

	lock := lockForArchive(a.options.Repository, name)
	lock.Lock()
	unlockOnError := true
	defer func() {
		if unlockOnError {
			lock.Unlock()
		}
	}()

	target, err := a.newTarget(ctx)
	if err != nil {
		return nil, err
	}

	path, err := os.MkdirTemp("", "radius-oci-")
	if err != nil {
		return nil, fmt.Errorf("failed to create archive directory: %w", err)
	}
	removePathOnError := true
	defer func() {
		if removePathOnError {
			if removeErr := os.RemoveAll(path); removeErr != nil {
				ucplog.FromContextOrDiscard(ctx).Info("Failed to remove archive directory", "path", path, "error", removeErr)
			}
		}
	}()

	session := &session{
		path:   path,
		name:   name,
		target: target,
		unlock: lock.Unlock,
	}

	desc, err := target.Resolve(ctx, name)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			unlockOnError = false
			removePathOnError = false
			return session, nil
		}
		return nil, fmt.Errorf("failed to resolve OCI archive %q: %w", name, err)
	}
	session.manifestDigest = desc.Digest

	if err := unpackArchive(ctx, target, desc, path); err != nil {
		return nil, fmt.Errorf("failed to unpack OCI archive %q: %w", name, err)
	}

	unlockOnError = false
	removePathOnError = false
	return session, nil
}

func (a *OCIArchive) openRemoteTarget(context.Context) (oras.Target, error) {
	if a.options.Repository == "" {
		return nil, errors.New("OCI archive repository is not configured; set RADIUS_STATE_REGISTRY or RADIUS_GRAPH_REGISTRY")
	}

	credentialStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
		AllowPlaintextPut: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read Docker credentials: %w", err)
	}

	target, err := remote.NewRepository(a.options.Repository)
	if err != nil {
		return nil, fmt.Errorf("invalid OCI archive repository %q: %w", a.options.Repository, err)
	}
	target.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Cache:      auth.DefaultCache,
		Credential: credentialStore.Get,
	}
	target.PlainHTTP = a.options.PlainHTTP
	return target, nil
}

func lockForArchive(repository, name string) *sync.Mutex {
	key := repository + ":" + name
	lock, _ := archiveLocks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

type session struct {
	path           string
	name           string
	target         oras.Target
	manifestDigest digest.Digest

	unlock   func()
	unlocked bool
}

// Path returns the session's temporary working directory.
func (s *session) Path() string {
	return s.path
}

// Commit writes the session directory to the configured OCI repository.
func (s *session) Commit(ctx context.Context, _ string) error {
	layer, changed, err := createLayer(s.path)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	source, manifestDesc, err := createArtifact(ctx, s.name, layer)
	if err != nil {
		return err
	}
	if manifestDesc.Digest == s.manifestDigest {
		return nil
	}

	if err := s.ensureTagUnchanged(ctx); err != nil {
		return err
	}

	if _, err := oras.Copy(ctx, source, s.name, s.target, s.name, oras.DefaultCopyOptions); err != nil {
		return fmt.Errorf("failed to push OCI archive %q: %w", s.name, err)
	}
	s.manifestDigest = manifestDesc.Digest
	return nil
}

func (s *session) ensureTagUnchanged(ctx context.Context) error {
	desc, err := s.target.Resolve(ctx, s.name)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) && s.manifestDigest == "" {
			return nil
		}
		return fmt.Errorf("failed to check OCI archive %q before push: %w", s.name, err)
	}
	if desc.Digest != s.manifestDigest {
		return fmt.Errorf("OCI archive %q changed while this session was open", s.name)
	}
	return nil
}

// Close removes the temporary directory and releases the session lock.
func (s *session) Close(ctx context.Context) {
	if err := os.RemoveAll(s.path); err != nil {
		ucplog.FromContextOrDiscard(ctx).Info("Failed to remove archive directory", "path", s.path, "error", err)
	}
	if !s.unlocked {
		s.unlocked = true
		s.unlock()
	}
}

func createArtifact(ctx context.Context, name string, layer []byte) (oras.Target, ocispec.Descriptor, error) {
	source := memory.New()

	layerDesc, err := pushBlob(ctx, source, layerMediaType, layer)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to add archive layer: %w", err)
	}
	configDesc, err := pushBlob(ctx, source, configMediaType, []byte("{}"))
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to add archive config: %w", err)
	}

	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: configMediaType,
		Config:       configDesc,
		Layers:       []ocispec.Descriptor{layerDesc},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to create archive manifest: %w", err)
	}
	manifestDesc, err := pushBlob(ctx, source, ocispec.MediaTypeImageManifest, manifestBytes)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to add archive manifest: %w", err)
	}
	if err := source.Tag(ctx, manifestDesc, name); err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to tag archive manifest: %w", err)
	}
	return source, manifestDesc, nil
}

func pushBlob(ctx context.Context, target oras.Target, mediaType string, data []byte) (ocispec.Descriptor, error) {
	desc := content.NewDescriptorFromBytes(mediaType, data)
	if err := target.Push(ctx, desc, bytes.NewReader(data)); err != nil {
		return ocispec.Descriptor{}, err
	}
	return desc, nil
}

func createLayer(root string) ([]byte, bool, error) {
	paths, err := archiveFiles(root)
	if err != nil {
		return nil, false, err
	}
	if len(paths) == 0 {
		return nil, false, nil
	}

	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	gzipWriter.Header.ModTime = time.Unix(0, 0)
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, false, fmt.Errorf("failed to determine archive path: %w", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, false, fmt.Errorf("failed to stat archive file %q: %w", rel, err)
		}
		header := &tar.Header{
			Format:  tar.FormatPAX,
			Name:    filepath.ToSlash(rel),
			Mode:    0o644,
			Size:    info.Size(),
			ModTime: time.Unix(0, 0),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, false, fmt.Errorf("failed to write archive header for %q: %w", rel, err)
		}

		file, err := os.Open(path)
		if err != nil {
			return nil, false, fmt.Errorf("failed to open archive file %q: %w", rel, err)
		}
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return nil, false, fmt.Errorf("failed to add archive file %q: %w", rel, copyErr)
		}
		if closeErr != nil {
			return nil, false, fmt.Errorf("failed to close archive file %q: %w", rel, closeErr)
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, false, fmt.Errorf("failed to finish archive: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, false, fmt.Errorf("failed to compress archive: %w", err)
	}
	return output.Bytes(), true, nil
}

func archiveFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("archive contains unsupported symbolic link %q", path)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("archive contains unsupported file type %q", path)
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read archive directory: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func unpackArchive(ctx context.Context, target oras.Target, manifestDesc ocispec.Descriptor, root string) error {
	manifestReader, err := target.Fetch(ctx, manifestDesc)
	if err != nil {
		return err
	}
	manifestBytes, readErr := io.ReadAll(manifestReader)
	closeErr := manifestReader.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("invalid OCI manifest: %w", err)
	}
	if len(manifest.Layers) != 1 || manifest.Layers[0].MediaType != layerMediaType {
		return errors.New("OCI archive manifest must contain exactly one state archive layer")
	}

	layerReader, err := target.Fetch(ctx, manifest.Layers[0])
	if err != nil {
		return err
	}

	gzipReader, err := gzip.NewReader(layerReader)
	if err != nil {
		_ = layerReader.Close()
		return fmt.Errorf("invalid archive compression: %w", err)
	}

	unpackErr := unpackArchiveEntries(gzipReader, root)
	gzipCloseErr := gzipReader.Close()
	layerCloseErr := layerReader.Close()
	if unpackErr != nil {
		return unpackErr
	}
	if gzipCloseErr != nil {
		return fmt.Errorf("failed to close archive compression stream: %w", gzipCloseErr)
	}
	if layerCloseErr != nil {
		return fmt.Errorf("failed to close archive layer: %w", layerCloseErr)
	}
	return nil
}

func unpackArchiveEntries(reader io.Reader, root string) error {
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("invalid archive entry: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			return fmt.Errorf("unsupported archive entry %q", header.Name)
		}

		path, err := archivePath(root, header.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("failed to create archive directory: %w", err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("failed to create archive file %q: %w", header.Name, err)
		}
		_, copyErr := io.Copy(file, tarReader)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("failed to unpack archive file %q: %w", header.Name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("failed to close archive file %q: %w", header.Name, closeErr)
		}
	}
}

func archivePath(root, name string) (string, error) {
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if cleanName == "." || filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid archive path %q", name)
	}
	path := filepath.Join(root, cleanName)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid archive path %q", name)
	}
	return path, nil
}

var (
	_ statearchive.Archive = (*OCIArchive)(nil)
	_ statearchive.Session = (*session)(nil)
)
