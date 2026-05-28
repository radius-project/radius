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
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/radius-project/radius/pkg/graph/persistence"
	"github.com/radius-project/radius/pkg/graph/serialize"
)

// Options configures a Store.
type Options struct {
	// Branch is the orphan branch used to persist payloads. If empty,
	// DefaultGraphBranch is used.
	Branch string
}

// Store is a persistence.Store backed by a git orphan branch. Each saved
// payload is committed as a file on the branch using a StateWorktree, so the
// application's working tree is never touched.
//
// Key → path layout on the branch:
//
//	<namespace>/<name>.json
//
// Key.Version is currently ignored by this backend.
//
// Concurrency: `git worktree add` refuses to add a second worktree for a
// branch that is already checked out, so concurrent Save/Load/List/Delete
// calls on the same Store would otherwise fail nondeterministically. The
// persistence.Store contract requires implementations to be safe for
// concurrent use, so all operations are serialized through mu.
type Store struct {
	branch string
	mu     sync.Mutex
}

// NewStore returns a git-backed Store. The repository is auto-detected via
// `git rev-parse --show-toplevel` at I/O time, so no path is required up
// front.
func NewStore(opts Options) (*Store, error) {
	branch := opts.Branch
	if branch == "" {
		branch = DefaultGraphBranch
	}
	return &Store{branch: branch}, nil
}

// Save commits payload to the configured orphan branch under pathFor(key).
func (s *Store) Save(ctx context.Context, key persistence.Key, payload *serialize.Payload, opts persistence.SaveOptions) error {
	if payload == nil {
		return errors.New("git: nil payload")
	}
	path, err := pathForKey(key)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := OpenOrCreate(ctx, s.branch)
	if err != nil {
		return err
	}
	defer wt.Remove(ctx)

	if err := wt.WriteFile(path, payload.Data); err != nil {
		return err
	}

	msg := opts.Message
	if msg == "" {
		msg = fmt.Sprintf("radius: update %s", path)
	}
	return wt.CommitAndPush(ctx, msg)
}

// Load returns the payload previously stored under key, or persistence.ErrNotFound.
func (s *Store) Load(ctx context.Context, key persistence.Key) (*serialize.Payload, error) {
	path, err := pathForKey(key)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := OpenOrCreate(ctx, s.branch)
	if err != nil {
		return nil, err
	}
	defer wt.Remove(ctx)

	data, err := wt.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, persistence.ErrNotFound
		}
		return nil, err
	}
	return &serialize.Payload{
		ContentType: "application/json",
		Data:        data,
	}, nil
}

// List returns keys present on the branch under namespace. An empty namespace
// lists every key on the branch.
func (s *Store) List(ctx context.Context, namespace string) ([]persistence.Key, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := OpenOrCreate(ctx, s.branch)
	if err != nil {
		return nil, err
	}
	defer wt.Remove(ctx)

	root := wt.Path
	if namespace != "" {
		root = filepath.Join(wt.Path, namespace)
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
		rel, err := filepath.Rel(wt.Path, path)
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
	path, err := pathForKey(key)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := OpenOrCreate(ctx, s.branch)
	if err != nil {
		return err
	}
	defer wt.Remove(ctx)

	if err := wt.RemoveFile(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return persistence.ErrNotFound
		}
		return err
	}

	msg := fmt.Sprintf("radius: delete %s", path)
	return wt.CommitAndPush(ctx, msg)
}

// pathFor returns the in-repo relative path used to store a payload for key.
// Key.Version is ignored. Callers handling untrusted Key values should use
// pathForKey, which additionally validates Namespace and Name.
func pathFor(key persistence.Key) string {
	return fmt.Sprintf("%s/%s.json", key.Namespace, key.Name)
}

// pathForKey is pathFor with a check that Key.Namespace and Key.Name are
// non-empty so that the resulting path is always rooted under a namespace
// directory on the branch.
func pathForKey(key persistence.Key) (string, error) {
	if key.Namespace == "" {
		return "", errors.New("git: key namespace must not be empty")
	}
	if key.Name == "" {
		return "", errors.New("git: key name must not be empty")
	}
	return pathFor(key), nil
}

// keyFromPath inverts pathFor for a relative posix path of the form
// "<namespace>/<name>.json".
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
