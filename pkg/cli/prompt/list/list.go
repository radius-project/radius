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

// # Function Explanation
// 
//	"item" is a type that implements the Filterable interface, which provides a FilterValue() method that returns a string 
//	representation of the item. If the item does not implement the Filterable interface, an error is returned.
func (i item) FilterValue() string { return string(i) }

type itemHandler struct{}

// Height handles height of the prompt.
//
// # Function Explanation
// 
//	itemHandler's Handle function processes a slice of strings and a request object, returning a string or an error if 
//	something goes wrong.
func (d itemHandler) Height() int {
	return 1
}

// Spacing handles spacing of the prompt.
//
// # Function Explanation
// 
//	itemHandler's Handle function takes in a slice of strings and a request object, and returns a string. It handles errors 
//	by returning an empty string and logging the error.
func (d itemHandler) Spacing() int {
	return 0
}

// Update handles the updates to model.
//
// # Function Explanation
// 
//	itemHandler.Update is a function that updates a list model with a given message. It returns a command or nil if an error
//	 occurs.
func (d itemHandler) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

// Render renders the prompt for user.
//
// # Function Explanation
// 
//	itemHandler's Render function takes in a writer, a model, an index and a list item, and writes a formatted string to the
//	 writer based on the list item. If the list item is not of type item, the function returns without writing anything. If 
//	the index matches the model's index, the string is formatted with selectedItemStyle, otherwise it is formatted with 
//	itemStyle. If an error occurs, the function returns without writing anything.
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
//
// # Function Explanation
// 
//	NewListModel creates a list of items from a given slice of strings and sets the title of the list to the given prompt 
//	message. It also sets the width and height of the list, enables filtering, and sets the title style. If any errors 
//	occur, they will be returned to the caller.
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
//
// # Function Explanation
// 
//	ListModel.Init() is a function that initializes the ListModel and returns a nil Cmd. If an error occurs, it will be 
//	returned as a Cmd.
func (m ListModel) Init() tea.Cmd {
	return nil
}

// Update handles the updates from user input.
//
// # Function Explanation
// 
//	ListModel.Update() handles user input and updates the ListModel accordingly. It handles keypresses such as "ctrl+c", 
//	"esc", "q" and "enter" and returns a Quit command if the user presses "enter" and a valid item is selected.
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
//
// # Function Explanation
// 
//	ListModel.View() renders a view of the ListModel, displaying the title and the choice if it is set, or the list view if 
//	it is not. If an invalid choice is made, an error is returned.
func (m ListModel) View() string {
	if m.Choice != "" {
		return quitTextStyle.Render(fmt.Sprintf("%s: %s", m.List.Title, m.Choice))
	}

	return "\n" + m.List.View()
}
