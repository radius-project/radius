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

// Package progress renders live, in-place progress for resource deployment and
// deletion. The interactive renderer is built on Bubble Tea and the Bubbles
// spinner component (both already used elsewhere in the CLI); when stdout is not
// a terminal it falls back to a no-op listener that simply drains the channel.
package progress

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-isatty"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/output"
)

// Listener renders progress updates received on a channel until the channel is
// closed. Run blocks and is intended to be called from a goroutine.
type Listener interface {
	// Run consumes updates from the progress channel until it is closed. This
	// should be called from a goroutine because it blocks.
	Run()
}

// NewListener returns an interactive spinner-based Listener when stdout is a
// terminal, or a no-op Listener that drains the channel otherwise.
func NewListener(progressChan <-chan clients.ResourceProgress) Listener {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		return &interactiveListener{progressChan: progressChan}
	}
	return &noOpListener{progressChan: progressChan}
}

type noOpListener struct {
	progressChan <-chan clients.ResourceProgress
}

// Run drains updates from the channel without taking any action.
func (l *noOpListener) Run() {
	for range l.progressChan {
		// Do nothing except drain the updates.
	}
}

type interactiveListener struct {
	progressChan <-chan clients.ResourceProgress
}

// Run renders progress in place until the channel is closed. It deliberately
// does not capture stdin or install a signal handler, preserving the previous
// behavior where Ctrl+C terminates the process during a deployment or deletion.
func (l *interactiveListener) Run() {
	program := tea.NewProgram(
		newModel(l.progressChan),
		tea.WithInput(strings.NewReader("")),
		tea.WithoutSignalHandler(),
	)
	if _, err := program.Run(); err != nil {
		// Bubble Tea failed to start or exited before the channel was closed.
		// The model is no longer reading from progressChan, so drain it here to
		// keep producers (deploy/delete) from blocking until it is closed.
		fmt.Fprintf(os.Stderr, "Warning: progress display stopped: %v\n", err)
		for range l.progressChan {
			// Drain remaining updates so writers do not block.
		}
	}
}

// progressMsg carries a single resource progress update into the Bubble Tea loop.
type progressMsg clients.ResourceProgress

// doneMsg signals that the progress channel has been closed.
type doneMsg struct{}

type entry struct {
	// format is a format string containing a single verb for the status token
	// (the spinner frame while in progress, or the final-state token).
	format string
	// finalState is empty while the resource is in progress, otherwise it holds
	// the terminal status token that replaces the spinner.
	finalState string
}

type model struct {
	updates <-chan clients.ResourceProgress
	spinner spinner.Model
	entries []entry
	// index maps a resource ID to its position in entries so repeated updates
	// for the same resource reuse the same line.
	index map[string]int
}

func newModel(updates <-chan clients.ResourceProgress) *model {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: output.ProgressDefaultSpinner,
		FPS:    time.Second / 2, //nolint:mnd // Matches the previous 500ms cadence.
	}
	return &model{
		updates: updates,
		spinner: s,
		index:   map[string]int{},
	}
}

// Init starts the spinner animation and the first read from the progress channel.
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitForUpdate(m.updates))
}

// waitForUpdate blocks on the next channel value and converts it into a message.
// When the channel is closed it emits a doneMsg so the program can quit.
func waitForUpdate(updates <-chan clients.ResourceProgress) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-updates
		if !ok {
			return doneMsg{}
		}
		return progressMsg(update)
	}
}

// Update handles progress updates, channel closure, and spinner ticks.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressMsg:
		m.apply(clients.ResourceProgress(msg))
		return m, waitForUpdate(m.updates)
	case doneMsg:
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// apply records or updates the entry for the resource referenced by the update.
// Resources can move directly to a terminal state without first being started.
func (m *model) apply(update clients.ResourceProgress) {
	if !output.ShowResource(update.Resource) {
		return
	}

	format := output.FormatResourceForProgressDisplay(update.Resource)
	key := update.Resource.String()
	i, ok := m.index[key]
	if !ok {
		m.entries = append(m.entries, entry{format: format})
		i = len(m.entries) - 1
		m.index[key] = i
	}

	switch update.Status {
	case clients.StatusFailed:
		m.entries[i] = entry{format: format, finalState: output.ProgressFailed}
	case clients.StatusCompleted:
		m.entries[i] = entry{format: format, finalState: output.ProgressCompleted}
	}
}

// View renders every tracked resource on its own line, replacing the spinner
// with a final-state token once the resource reaches a terminal status.
func (m *model) View() tea.View {
	var b strings.Builder
	for _, e := range m.entries {
		token := m.spinner.View()
		if e.finalState != "" {
			token = e.finalState
		}
		fmt.Fprintf(&b, e.format+"\n", token)
	}
	return tea.NewView(b.String())
}
