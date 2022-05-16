// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testcontext

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
)

func New(t *testing.T) (context.Context, context.CancelFunc) {
	ctx := logr.NewContext(context.Background(), testr.New(t))
	deadline, ok := t.Deadline()
	if ok {
		return context.WithDeadline(ctx, deadline)
	} else {
		return context.WithCancel(ctx)
	}
}
