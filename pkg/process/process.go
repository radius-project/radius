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
package process

import (
	"context"
	"os/exec"
)

// Command returns the Cmd to execute the named program with the given arguments.
func Command(name string, args ...string) *exec.Cmd {
	return configure(exec.Command(name, args...))
}

// CommandContext returns the Cmd to execute the named program with the given arguments.
// The provided context controls the command lifetime.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return configure(exec.CommandContext(ctx, name, args...))
}
