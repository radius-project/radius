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

package resource_test

import (
	"log"
	"os/exec"
	"testing"

	expect "github.com/Netflix/go-expect"
	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/stretchr/testify/require"
)

// Prompt provides test utility for prompt test.
type Prompt struct {
	CmdArgs   []string
	Procedure func(*testing.T, *expect.Console) error
}

type InteractiveCommandRunner struct {
}

func NewInteractiveCommandRunner() *InteractiveCommandRunner {
	return &InteractiveCommandRunner{}
}

func (r *InteractiveCommandRunner) Run(t *testing.T, prompt Prompt) {
	t.Helper()

	// Prepare the pseudo-terminal.
	//
	// pty is the main part of the pseudo-terminal.
	// > It is what your application will interact with when it wants to control the terminal.
	// > Reading from pty gets the output of the terminal, and writing to pty writes input to the terminal.
	// tty is the other end of the terminal.
	// > It is what you would see on the screen in a terminal application.
	pty, tty, err := pty.Open()
	require.NoError(t, err)
	defer func() { _ = pty.Close() }() // Close pty when done.
	defer func() { _ = tty.Close() }() // Close tty when done.

	// Create a terminal emulator.
	term := vt10x.New(vt10x.WithWriter(tty))

	// Create a console.
	c, err := expect.NewConsole(
		expect.WithStdin(pty),
		expect.WithStdout(term),
		expect.WithCloser(pty, tty),
	)
	require.NoError(t, err)
	defer c.Close() // Close console when done.

	// Prepare and start command on terminal emulator
	cmd := exec.Command("rad", prompt.CmdArgs...)
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty

	go func() {
		if err := prompt.Procedure(t, c); err != nil {
			t.Errorf("procedure failed: %v", err)
		}
	}()

	err = cmd.Start()
	if err != nil {
		log.Panicf("cmd.Start() = %v", err)
	}

	err = cmd.Wait()
	if err != nil {
		log.Panicf("cmd.Wait() = %v", err)
	}
}

func TestInteractiveRadiusCommands(t *testing.T) {
	// t.Parallel()

	runner := NewInteractiveCommandRunner()

	t.Run("rad init with application to be scaffolded", func(t *testing.T) {
		runner.Run(t, Prompt{
			CmdArgs: []string{"init"},
			Procedure: func(t *testing.T, c *expect.Console) error {
				_, err := c.ExpectString("Setup application in the current directory?")
				require.NoError(t, err)

				// Trying to send Enter key press.
				_, err = c.SendLine("")
				require.NoError(t, err)

				return nil
			},
		})
	})

	t.Run("rad init with application not to be scaffolded", func(t *testing.T) {
		runner.Run(t, Prompt{
			CmdArgs: []string{"init"}, // Assuming that "--yes" is a valid argument for "rad init"
			Procedure: func(t *testing.T, c *expect.Console) error {
				_, err := c.ExpectString("Setup application in the current directory?")
				require.NoError(t, err)

				// `j` is equal to CursorDown.
				_, err = c.Send("j")
				require.NoError(t, err)

				// Trying to send Enter key press.
				_, err = c.SendLine("")
				require.NoError(t, err)

				return nil
			},
		})
	})
}
