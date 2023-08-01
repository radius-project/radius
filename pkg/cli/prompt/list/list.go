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

package list

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const listHeight = 14

const defaultWidth = 400

var (
	titleStyle        = lipgloss.NewStyle().PaddingLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#555555", Dark: "#AAAAAA"})
)

type item string

// # Function Explanation
//
// FilterValue takes in an item and returns a string representation of it.
func (i item) FilterValue() string { return string(i) }

type itemHandler struct{}

// Height handles height of the prompt.
func (d itemHandler) Height() int {
	return 1
}

// Spacing handles spacing of the prompt.
func (d itemHandler) Spacing() int {
	return 0
}

// Update handles the updates to model.
func (d itemHandler) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

// Render renders the prompt for user.
//
// # Function Explanation
//
// Render takes in a writer, a model, an index and a list item, and writes a formatted string to the writer
// based on the index and list item. If the list item is not of the expected type, it returns without writing anything.
func (d itemHandler) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render(append([]string{"> "}, s...)...)
		}
	}

	fmt.Fprint(w, fn(str))
}

// NewListModel returns a list model for bubble tea prompt.
//
// # Function Explanation
//
// NewListModel creates a ListModel struct with a list of items, a title, and styling for the list and help.
func NewListModel(choices []string, promptMsg string) ListModel {
	items := make([]list.Item, len(choices))
	for i, choice := range choices {
		items[i] = item(choice)
	}

	l := list.New(items, itemHandler{}, defaultWidth, listHeight)
	l.Title = promptMsg
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	// The built-in help styles (list of keybindings) dont't have enough contrast.
	l.Help.Styles.FullKey = helpStyle
	l.Help.Styles.ShortKey = helpStyle
	l.Help.Styles.FullDesc = helpStyle
	l.Help.Styles.ShortDesc = helpStyle

	return ListModel{
		List:  l,
		Style: lipgloss.NewStyle(),
	}
}

// ListMode represents the bubble tea model to use for user input
type ListModel struct {
	List   list.Model
	Choice string
	// Style configures the style applied to all rendering for the list. This can be used to apply padding and borders.
	Style    lipgloss.Style
	Quitting bool
}

// Init used for creating an inital tea command if needed.
func (m ListModel) Init() tea.Cmd {
	return nil
}

// Update handles the updates from user input.
//
// # Function Explanation
//
// Update() handles user input and updates the list model accordingly, allowing the user to select an item from
// the list and quit the application.
func (m ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "q":
			m.Quitting = true
			return m, tea.Quit
		case "enter":
			if m.List.FilterState() != list.Filtering {
				i, ok := m.List.SelectedItem().(item)
				if ok {
					m.Choice = string(i)
				}
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.List, cmd = m.List.Update(msg)
	return m, cmd
}

// View renders the view after user selection.
func (m ListModel) View() string {
	if m.Choice != "" {
		// Hide output once the choice has been made
		return ""
	}

	return m.Style.Render(m.List.View())
}
