// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package step

import (
	"context"
	"testing"

	"github.com/project-radius/radius/test"
)

type Executor interface {
	GetDescription() string
	Execute(ctx context.Context, t *testing.T, options test.TestOptions)
}
