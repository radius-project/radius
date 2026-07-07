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

// Package git provides a persistence.Store backed by a git orphan branch.
//
// It is a thin, graph-specific adapter over the shared pluggable storage
// backend in pkg/statearchive: it maps persistence.Key values to JSON files on the
// orphan branch and delegates all git I/O to a statearchive.Archive (the git one by
// default). Swapping in a different statearchive.Archive (for example an OCI or
// filesystem implementation) requires no change here.
//
// Key -> path layout on the branch:
//
//	<namespace>/<name>.json
package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/graph/persistence"
	"github.com/radius-project/radius/pkg/statearchive"
	archivegit "github.com/radius-project/radius/pkg/statearchive/git"
)

// DefaultGraphBranch is the default orphan branch name for graph artifacts.
const DefaultGraphBranch = "radius-graph"

// Options configures a Store.
type Options struct {
	// Branch is the orphan branch used to persist graphs. If empty,
	// DefaultGraphBranch is used.
	Branch string

	// Archive is the durable state archive. If nil, the git orphan-branch
	// implementation is used. Tests may inject an alternative implementation.
	Archive statearchive.Archive
}

// Store is a persistence.Store that persists each graph as a JSON file in a
// durable state archive. The default archive is a git orphan branch, so the
// application's working tree is never touched.
//
// Concurrency: the persistence.Store contract requires implementations to be
// safe for concurrent use. Serialization is delegated to the archive, which
// serializes sessions per branch (git refuses two worktrees on one branch).
type Store struct {
	branch  string
	archive statearchive.Archive
}

// NewStore returns a git-backed Store. The repository is auto-detected by the
// archive at I/O time, so no path is required up front.
func NewStore(opts Options) (*Store, error) {
	branch := opts.Branch
	if branch == "" {
		branch = DefaultGraphBranch
	}
	archive := opts.Archive
	if archive == nil {
		archive = archivegit.NewGitArchive()
	}
	return &Store{branch: branch, archive: archive}, nil
}

// Save commits graph to the configured branch under constructPathForKey(key).
func (s *Store) Save(ctx context.Context, key persistence.Key, graph *corerpv20250801preview.ApplicationGraphResponse, opts persistence.SaveOptions) error {
	if graph == nil {
		return errors.New("git: nil graph")
	}
	path, err := constructPathForKey(key)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("git: marshal graph: %w", err)
	}

	session, err := s.archive.Open(ctx, s.branch)
	if err != nil {
		return err
	}
	defer session.Close(ctx)

	fullPath := filepath.Join(session.Path(), path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("git: creating directories for %s: %w", path, err)
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return fmt.Errorf("git: writing %s: %w", path, err)
	}

	msg := opts.Message
	if msg == "" {
		msg = fmt.Sprintf("radius: update %s", path)
	}
	return session.Commit(ctx, msg)
}

// Load returns the graph previously stored under key, or persistence.ErrNotFound.
func (s *Store) Load(ctx context.Context, key persistence.Key) (*corerpv20250801preview.ApplicationGraphResponse, error) {
	path, err := constructPathForKey(key)
	if err != nil {
		return nil, err
	}

	session, err := s.archive.Open(ctx, s.branch)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	data, err := os.ReadFile(filepath.Join(session.Path(), path))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, persistence.ErrNotFound
		}
		return nil, err
	}
	graph := &corerpv20250801preview.ApplicationGraphResponse{}
	if err := json.Unmarshal(data, graph); err != nil {
		return nil, fmt.Errorf("git: unmarshal graph at %s: %w", path, err)
	}
	return graph, nil
}

// List returns keys present on the branch under namespace. An empty namespace
// lists every key on the branch.
func (s *Store) List(ctx context.Context, namespace string) ([]persistence.Key, error) {
	if namespace != "" {
		if err := validateKeyPart("namespace", namespace); err != nil {
			return nil, err
		}
	}

	session, err := s.archive.Open(ctx, s.branch)
	if err != nil {
		return nil, err
	}
	defer session.Close(ctx)

	root := session.Path()
	if namespace != "" {
		root = filepath.Join(session.Path(), namespace)
	}

	var keys []persistence.Key
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipDir
			}
			return err
		}
		if d.IsDir() {
			// Skip the .git pointer file directory inside a worktree.
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		rel, err := filepath.Rel(session.Path(), path)
		if err != nil {
			return err
		}
		keys = append(keys, keyFromPath(filepath.ToSlash(rel)))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return keys, nil
}

// Delete removes the file for key and commits the deletion.
func (s *Store) Delete(ctx context.Context, key persistence.Key) error {
	path, err := constructPathForKey(key)
	if err != nil {
		return err
	}

	session, err := s.archive.Open(ctx, s.branch)
	if err != nil {
		return err
	}
	defer session.Close(ctx)

	if err := os.Remove(filepath.Join(session.Path(), path)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return persistence.ErrNotFound
		}
		return err
	}

	msg := fmt.Sprintf("radius: delete %s", path)
	return session.Commit(ctx, msg)
}

// constructPathForKey returns the in-repo relative path used to store a
// graph for key, after validating that Key.Namespace and Key.Name are safe
// to embed in a path. The resulting path is always rooted under a single
// namespace directory on the branch:
//
//	<namespace>/<name>.json
func constructPathForKey(key persistence.Key) (string, error) {
	if err := validateKeyPart("namespace", key.Namespace); err != nil {
		return "", err
	}
	if err := validateKeyPart("name", key.Name); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s.json", key.Namespace, key.Name), nil
}

// validateKeyPart rejects empty values and any value that could escape the
// intended namespace directory (path separators, parent-directory tokens, or
// embedded NUL bytes). The field name is included in the error so callers
// can tell which part of the key was invalid.
func validateKeyPart(field, value string) error {
	switch {
	case value == "":
		return fmt.Errorf("git: key %s must not be empty", field)
	case value == "." || value == "..":
		return fmt.Errorf("git: key %s must not be %q (path traversal)", field, value)
	case strings.ContainsAny(value, `/\`):
		return fmt.Errorf("git: key %s must not contain path separators", field)
	case strings.Contains(value, "\x00"):
		return fmt.Errorf("git: key %s must not contain NUL bytes", field)
	}
	return nil
}

// keyFromPath inverts constructPathForKey for a relative posix path of
// the form "<namespace>/<name>.json".
func keyFromPath(rel string) persistence.Key {
	parts := strings.Split(rel, "/")
	if len(parts) == 2 {
		return persistence.Key{
			Namespace: parts[0],
			Name:      strings.TrimSuffix(parts[1], ".json"),
		}
	}
	return persistence.Key{Name: strings.TrimSuffix(rel, ".json")}
}

// Compile-time check that *Store satisfies persistence.Store.
var _ persistence.Store = (*Store)(nil)
