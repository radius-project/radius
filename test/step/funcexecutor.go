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

package step

import (
	"context"
	"testing"

	"github.com/radius-project/radius/test"
)

var _ Executor = (*FuncExecutor)(nil)

// FuncExecutor implements the Executor interface. It encapsulates a function to be executed in a test step
// and includes a description.
type FuncExecutor struct {
	fn          func(ctx context.Context, t *testing.T, opts test.TestOptions)
	Description string
}

// Execute runs the encapsulated function with the provided context, testing instance, and test options.
func (f FuncExecutor) Execute(ctx context.Context, t *testing.T, opts test.TestOptions) {
	f.fn(ctx, t, opts)
}

// NewFuncExecutor initializes and returns a new FuncExecutor with the given function and a default description.
func NewFuncExecutor(fn func(ctx context.Context, t *testing.T, opts test.TestOptions)) FuncExecutor {
	return FuncExecutor{fn: fn,
		Description: "execute function in test step",
	}
}

// GetDescription returns the description of the test step encapsulated by the FuncExecutor.
func (f FuncExecutor) GetDescription() string {
	return f.Description
}
