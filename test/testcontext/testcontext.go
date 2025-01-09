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

package testcontext

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
)

// New creates a new context with test logger for testing.
func New(t *testing.T) context.Context {
	ctx, _ := Wrap(t, context.Background())
	return ctx
}

// NewWithCancel creates a new cancellable context with test logger for testing.
func NewWithCancel(t *testing.T) (context.Context, context.CancelFunc) {
	return Wrap(t, context.Background())
}

// NewWithDeadline creates a new deadline context based on the given duration, with test logger for testing.
func NewWithDeadline(t *testing.T, duration time.Duration) (context.Context, context.CancelFunc) {
	ctx, _ := Wrap(t, context.Background())
	return context.WithDeadline(ctx, time.Now().Add(duration))
}

// Wrap wraps a context with test logger for testing and returns the context with cancel function.
func Wrap(t *testing.T, ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}

	// See comments on testSink for why we need this.
	wrapper := &testSink{wrapped: t}
	t.Cleanup(wrapper.Stop)

	// Setting verbosity so that everything gets logged.
	ctx = logr.NewContext(ctx, testr.NewWithInterface(wrapper, testr.Options{LogTimestamp: true, Verbosity: 10000}))
	deadline, ok := t.Deadline()
	if ok {
		return context.WithDeadline(ctx, deadline)
	} else {
		return context.WithCancel(ctx)
	}
}

// testSink wraps testing.T for logging purposes.
//
// This type exists to work around an intentional limitation of testing.T.Log. It's not allowed to log after
// the test completes.
//
// Unfortunately that's not always viable for us given the amount of async processing we do in Radius.
//
// This type can hook onto testing.T.Cleanup and no-op all logging when the test completes.
type testSink struct {
	wrapped *testing.T
	mutex   sync.Mutex
	stopped bool
}

var _ testr.TestingT = (*testSink)(nil)

// Stop is called when the test terminates, this will disable logging.
func (t *testSink) Stop() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.stopped = true
}

// Helper implements testr.TestingT.
func (t *testSink) Helper() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.stopped {
		t.wrapped.Helper()
	}
}

// Log implements testr.TestingT.
func (t *testSink) Log(args ...any) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.stopped {
		t.wrapped.Log(args...)
	}
}
