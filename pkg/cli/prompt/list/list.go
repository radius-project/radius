// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

type item string

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
func (d itemHandler) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s string) string {
			return selectedItemStyle.Render("> " + s)
		}
	}

	fmt.Fprint(w, fn(str))
}

// NewListModel returns a list model for bubble tea prompt.
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

	return ListModel{
		List: l,
	}
}

// ListMode represents the bubble tea model to use for user input
type ListModel struct {
	List     list.Model
	Choice   string
	Quitting bool
}

// Init used for creating an inital tea command if needed.
func (m ListModel) Init() tea.Cmd {
	return nil
}

// Update handles the updates from user input.
func (m ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "esc", "q":
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
		return quitTextStyle.Render(fmt.Sprintf("%s: %s", m.List.Title, m.Choice))
	}

	return "\n" + m.List.View()
}
