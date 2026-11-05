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

package nativetest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

// errFailNow stops a Lifecycle run that fakeTB failed, as testing.T's FailNow does.
var errFailNow = errors.New("FailNow")

// fakeTB is a testing.TB that records failures and cleanups instead of failing the real test, so a test can check
// what a failed Lifecycle run leaves behind.
type fakeTB struct {
	testing.TB
	ctx      context.Context
	cancel   context.CancelFunc
	failed   bool
	errors   []string
	cleanups []func()
}

func newFakeTB(t *testing.T) *fakeTB {
	ctx, cancel := context.WithCancel(t.Context())
	return &fakeTB{TB: t, ctx: ctx, cancel: cancel}
}

func (f *fakeTB) Helper()                           {}
func (f *fakeTB) Context() context.Context          { return f.ctx }
func (f *fakeTB) Failed() bool                      { return f.failed }
func (f *fakeTB) Cleanup(fn func())                 { f.cleanups = append(f.cleanups, fn) }
func (f *fakeTB) Logf(format string, args ...any)   {}
func (f *fakeTB) Fatalf(format string, args ...any) { f.Errorf(format, args...); f.FailNow() }
func (f *fakeTB) FailNow()                          { f.failed = true; panic(errFailNow) }

func (f *fakeTB) Errorf(format string, args ...any) {
	f.failed = true
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}

// run runs fn as a test body, then cancels the context and runs the cleanups last first, as the testing package does.
func (f *fakeTB) run(fn func()) {
	func() {
		defer func() {
			if r := recover(); r != nil && r != errFailNow {
				panic(r)
			}
		}()
		fn()
	}()
	f.cancel()
	for _, cleanup := range slices.Backward(f.cleanups) {
		cleanup()
	}
}

// doneOnDelete reports every delete as done at once, as a handler can before AWS no longer has the resource.
type doneOnDelete struct {
	*nativefake.Handler
}

func (d doneOnDelete) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationDelete {
		return native.Progress{Done: true}, nil
	}
	return d.Handler.Progress(ctx, region, identifier, op)
}

func Test_Lifecycle_Cleanup(t *testing.T) {
	const typeName = "AWS::Test::Recorded"
	newRun := func(t *testing.T, recording bool) (Lifecycle, *nativefake.Handler, *awsfake.CloudControl) {
		file := filepath.Join(t.TempDir(), "cassette.json")
		if recording {
			t.Setenv(awsreplay.RecordEnv, "1")
		} else {
			t.Setenv(awsreplay.RecordEnv, "")
			require.NoError(t, os.WriteFile(file, []byte(`{"source": "recorded", "interactions": []}`), 0o600))
		}
		store := awsfake.NewCloudControl()
		store.Identifiers[typeName] = "Name"
		h := nativefake.New(typeName, store)
		l := Lifecycle{Handler: h, Session: awsreplay.New(t, file), Properties: map[string]any{"Name": "app"}, Poll: time.Millisecond}
		return l, h, store
	}
	deletes := func(h *nativefake.Handler) int {
		n := 0
		for _, c := range h.Calls() {
			if c.Method == "Delete" {
				n++
			}
		}
		return n
	}

	t.Run("a failed recording deletes the resource in cleanup", func(t *testing.T) {
		l, h, store := newRun(t, true)
		f := newFakeTB(t)
		l.Between = func() { f.Fatalf("boom") }
		f.run(func() { l.Run(f) })
		require.True(t, f.failed)
		require.Equal(t, 1, deletes(h))
		require.Zero(t, store.Count(typeName))
	})

	t.Run("a successful recording deletes once", func(t *testing.T) {
		l, h, store := newRun(t, true)
		f := newFakeTB(t)
		f.run(func() { l.Run(f) })
		require.False(t, f.failed, f.errors)
		require.Equal(t, 1, deletes(h))
		require.Zero(t, store.Count(typeName))
	})

	t.Run("a cleanup delete that fails says so", func(t *testing.T) {
		l, h, store := newRun(t, true)
		f := newFakeTB(t)
		l.Between = func() {
			h.Errors["Delete"] = errors.New("throttled")
			f.Fatalf("boom")
		}
		f.run(func() { l.Run(f) })
		require.Len(t, f.errors, 2)
		require.Contains(t, f.errors[1], "failed to delete app after the recording failed")
		require.Equal(t, 1, store.Count(typeName))
	})

	t.Run("a cleanup delete that AWS reports as failed says so", func(t *testing.T) {
		l, h, _ := newRun(t, true)
		f := newFakeTB(t)
		l.Between = func() {
			h.Failures[native.OperationDelete] = &native.Failure{Message: "the cluster has deletion protection"}
			f.Fatalf("boom")
		}
		f.run(func() { l.Run(f) })
		require.Len(t, f.errors, 2)
		require.Contains(t, f.errors[1], "deletion protection")
	})

	t.Run("a cleanup that can't read the delete's progress says so", func(t *testing.T) {
		l, h, _ := newRun(t, true)
		f := newFakeTB(t)
		l.Between = func() {
			h.Errors["Progress"] = errors.New("unavailable")
			f.Fatalf("boom")
		}
		f.run(func() { l.Run(f) })
		require.Len(t, f.errors, 2)
		require.Contains(t, f.errors[1], "unavailable")
		require.Equal(t, 15*time.Second, Lifecycle{}.poll(), "the default poll interval")
	})

	t.Run("a resource already gone needs no cleanup delete", func(t *testing.T) {
		l, h, store := newRun(t, true)
		f := newFakeTB(t)
		l.Between = func() {
			store.Remove(typeName, "app")
			f.Fatalf("boom")
		}
		f.run(func() { l.Run(f) })
		require.Len(t, f.errors, 1)
		require.Equal(t, 1, deletes(h))
	})

	t.Run("a cleanup stops when the delete reports done", func(t *testing.T) {
		l, h, store := newRun(t, true)
		l.Handler = doneOnDelete{h}
		f := newFakeTB(t)
		l.Between = func() { f.Fatalf("boom") }
		f.run(func() { l.Run(f) })
		require.Len(t, f.errors, 1)
		require.Equal(t, 1, deletes(h))
		require.Equal(t, 1, store.Count(typeName), "AWS said the delete is done before it removed the resource")
	})

	t.Run("the cleanup stops at its bound", func(t *testing.T) {
		l, _, _ := newRun(t, true)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := l.Handler.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: l.Properties, ClientToken: "token"})
		require.NoError(t, err)
		require.ErrorIs(t, l.deleteAndWait(ctx, "app"), context.Canceled)
	})

	t.Run("SkipDelete leaves the resource", func(t *testing.T) {
		l, h, store := newRun(t, true)
		l.SkipDelete = true
		f := newFakeTB(t)
		l.Between = func() { f.Fatalf("boom") }
		f.run(func() { l.Run(f) })
		require.Zero(t, deletes(h))
		require.Equal(t, 1, store.Count(typeName))
	})

	t.Run("a replay registers no cleanup", func(t *testing.T) {
		l, h, _ := newRun(t, false)
		f := newFakeTB(t)
		l.Between = func() { f.Fatalf("boom") }
		f.run(func() { l.Run(f) })
		require.Empty(t, f.cleanups)
		require.Zero(t, deletes(h))
	})
}
