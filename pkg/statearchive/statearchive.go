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

// Package statearchive defines a pluggable archive for durable Radius state
// that is exported out of a running cluster and restored later (for example
// across ephemeral CI runs).
//
// It is intentionally distinct from the live, record-oriented persistence
// subsystems in pkg/components (database.Client, secret.Client, queue.Client):
// those serve the running control plane, whereas an Archive captures a whole
// directory of state as a durable snapshot. Today the only implementation is a
// git orphan branch (pkg/statearchive/git), but the interface deliberately
// hides that: a Session is just a local working directory whose contents
// survive across Open calls once Commit succeeds. Callers write files into
// Session.Path() with any tool (pg_dump, kubectl, os.WriteFile, ...), then
// Commit to persist them. Alternative implementations (for example OCI/GHCR or
// a plain filesystem) implement the same two interfaces without changing any
// caller.
//
// Typical use:
//
//	session, err := archive.Open(ctx, "radius-state")
//	if err != nil {
//		return err
//	}
//	defer session.Close(ctx)
//	// ... read/write files under session.Path() ...
//	if err := session.Commit(ctx, "radius: backup"); err != nil {
//		return err
//	}
package statearchive

import "context"

// Archive is a pluggable durable state archive. Each named archive is
// materialized into a local working directory (a Session) that callers mutate
// with any tool and then persist atomically via Session.Commit.
//
// Implementations must be safe for concurrent use by multiple goroutines. An
// implementation is free to serialize concurrent Open calls for the same name
// when its underlying storage cannot support simultaneous sessions (the git
// implementation does this because git refuses two worktrees on one branch).
//
//go:generate go tool mockgen -typed -destination=./mock_archive.go -package=statearchive -self_package github.com/radius-project/radius/pkg/statearchive github.com/radius-project/radius/pkg/statearchive Archive,Session
type Archive interface {
	// Open materializes the durable archive identified by name into a local
	// working directory and returns a Session. Files persisted by a previous
	// Commit are present under Session.Path() when Open returns. The caller
	// must always defer Session.Close.
	Open(ctx context.Context, name string) (Session, error)
}

// Session is a durable working directory. Callers read and write files under
// Path using ordinary filesystem operations, Commit persists every change made
// under Path, and Close releases any resources the session holds.
type Session interface {
	// Path is the absolute path of the local working directory backing the
	// session. It is stable for the lifetime of the session.
	Path() string

	// Commit persists every change made under Path since Open (or the previous
	// Commit) to durable storage. When there is nothing to persist it is a
	// no-op. A Commit either durably persists the state or returns an error;
	// it never silently drops changes.
	Commit(ctx context.Context, message string) error

	// Close releases the session's resources. It is best-effort cleanup and is
	// safe to call from a deferred statement; failures are logged rather than
	// returned so they cannot mask the real error on the happy path.
	Close(ctx context.Context)
}
