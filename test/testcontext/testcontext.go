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

	ctx = logr.NewContext(ctx, testr.New(t))
	deadline, ok := t.Deadline()
	if ok {
		return context.WithDeadline(ctx, deadline)
	} else {
		return context.WithCancel(ctx)
	}
}
