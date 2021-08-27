// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testcontext

import (
	"context"
	"testing"
)

func GetContext(t *testing.T) (context.Context, context.CancelFunc) {
	var ctx context.Context
	var cancel context.CancelFunc
	deadline, ok := t.Deadline()
	if ok {
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	return ctx, cancel
}
