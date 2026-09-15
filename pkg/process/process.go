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

// Package process constructs external commands using Radius process policies.
//
// In Windows windowless mode, commands default to explicit EOF on stdin. Go's
// exec.Cmd already connects nil stdin to the null device; this policy makes that
// default explicit, not a guarantee that arbitrary tools or SDK-owned credential
// helpers cannot prompt. Callers may still assign finite input to Cmd.Stdin.
package process

import (
	"context"
	"os/exec"
)

// IsWindowless reports whether Radius is running on Windows without an attached
// console. Tool adapters can use it to select their own non-interactive behavior.
// It checks console attachment when called, not during package initialization,
// and returns false on non-Windows platforms. It does not detect redirected
// streams or a general cross-platform automation mode.
func IsWindowless() bool {
	return isWindowless()
}

// Command returns the Cmd to execute the named program with the given arguments.
// In windowless mode it defaults stdin to EOF; callers may replace Cmd.Stdin.
func Command(name string, args ...string) *exec.Cmd {
	return configure(exec.Command(name, args...))
}

// CommandContext returns the Cmd to execute the named program with the given arguments.
// The provided context controls the command lifetime.
// In windowless mode it defaults stdin to EOF; callers may replace Cmd.Stdin.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return configure(exec.CommandContext(ctx, name, args...))
}
