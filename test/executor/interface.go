// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package executor

import (
	"context"
	"testing"

	"github.com/project-radius/radius/test"
)

type StepExecutor interface {
	GetDescription() string
	Execute(ctx context.Context, t *testing.T, options test.TestOptions)
}
