//go:build windows

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

package process

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestCommands_ConsolePolicy(t *testing.T) {
	// These subtests change the shared console hook and must remain sequential.
	for _, tt := range []struct {
		name     string
		attached bool
	}{
		{name: "attached", attached: true},
		{name: "windowless", attached: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setHasConsole(t, tt.attached)
			require.Equal(t, !tt.attached, IsWindowless())

			for _, constructor := range commandConstructors {
				t.Run(constructor.name, func(t *testing.T) {
					cmd := constructor.command("test-command", "argument")
					require.Equal(t, []string{"test-command", "argument"}, cmd.Args)
					if tt.attached {
						require.Nil(t, cmd.SysProcAttr)
						require.Nil(t, cmd.Stdin)
					} else {
						require.NotNil(t, cmd.SysProcAttr)
						require.True(t, cmd.SysProcAttr.HideWindow)
						require.Equal(t, uint32(windows.CREATE_NO_WINDOW), cmd.SysProcAttr.CreationFlags)
						require.NotNil(t, cmd.Stdin)
						n, err := cmd.Stdin.Read(make([]byte, 1))
						require.Zero(t, n)
						require.ErrorIs(t, err, io.EOF)
					}
				})
			}

			testCommandInput(t)
			t.Run("context cancellation", testCommandContextCancellation)
		})
	}
}

func TestConfigure_PreservesCallerSettings(t *testing.T) {
	for _, tt := range []struct {
		name     string
		attached bool
	}{
		{name: "attached", attached: true},
		{name: "windowless", attached: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setHasConsole(t, tt.attached)
			cmd := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
			input := bytes.NewReader([]byte("SELECT 1;\n"))
			cmd.Stdin = input
			attrs := &syscall.SysProcAttr{
				CreationFlags:              windows.CREATE_NEW_PROCESS_GROUP,
				CmdLine:                    "caller command line",
				Token:                      syscall.Token(123),
				NoInheritHandles:           true,
				AdditionalInheritedHandles: []syscall.Handle{456},
				ParentProcess:              syscall.Handle(789),
			}
			cmd.SysProcAttr = attrs
			expected := *attrs
			if !tt.attached {
				expected.HideWindow = true
				expected.CreationFlags |= windows.CREATE_NO_WINDOW
			}

			require.Same(t, cmd, configure(cmd))
			require.Same(t, attrs, cmd.SysProcAttr)
			require.Equal(t, expected, *cmd.SysProcAttr)
			require.Same(t, input, cmd.Stdin)
			require.Zero(t, cmd.SysProcAttr.CreationFlags&uint32(windows.DETACHED_PROCESS|windows.CREATE_BREAKAWAY_FROM_JOB))
		})
	}
}

func setHasConsole(t *testing.T, attached bool) {
	t.Helper()
	original := hasConsole
	hasConsole = func() bool {
		return attached
	}
	t.Cleanup(func() {
		hasConsole = original
	})
}
