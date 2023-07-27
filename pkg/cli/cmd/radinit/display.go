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

package radinit

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/project-radius/radius/pkg/cli/prompt"
)

const (
	summaryIndent                                 = "   - "
	summaryHeading                                = "You've selected the following:\n\n"
	summaryFooter                                 = "\n(press enter to confirm or esc to restart)\n"
	summaryKubernetesHeadingIcon                  = "🔧 "
	summaryKubernetesInstallHeadingFmt            = "Install Radius %s\n" + summaryIndent + "Kubernetes cluster: %s\n" + summaryIndent + "Kubernetes namespace: %s\n"
	summaryKubernetesInstallAWSCloudProviderFmt   = summaryIndent + "AWS IAM access key id: %s\n"
	summaryKubernetesInstallAzureCloudProviderFmt = summaryIndent + "Azure service principal: %s\n"
	summaryKubernetesExistingHeadingFmt           = "Use existing Radius %s install on %s\n"
	summaryEnvironmentHeadingIcon                 = "🌏 "
	summaryEnvironmentCreateHeadingFmt            = "Create new environment %s\n" + summaryIndent + "Kubernetes namespace: %s\n"
	summaryEnvironmentCreateAWSCloudProviderFmt   = summaryIndent + "AWS: account %s and region %s\n"
	summaryEnvironmentCreateAzureCloudProviderFmt = summaryIndent + "Azure: subscription %s and resource group %s\n"
	summaryEnvironmentCreateRecipePackyFmt        = summaryIndent + "Recipe pack: %s\n"
	summaryEnvironmentExistingHeadingFmt          = "Use existing environment %s\n"
	summaryApplicationHeadingIcon                 = "🚧 "
	summaryApplicationScaffoldHeadingFmt          = "Scaffold application %s\n"
	summaryApplicationScaffoldFile                = summaryIndent + "Create %s\n"
	summaryConfigurationHeadingIcon               = "📋 "
	summaryConfigurationUpdateHeading             = "Update local configuration\n"
	progressHeading                               = "Initializing Radius...\n\n"
	progressCompleteFooter                        = "\nInitialization complete! Have a RAD time 😎\n\n"
	progressStepCompleteIcon                      = "✅ "
	progressStepWaitingIcon                       = "⏳ "
)

var (
	progressSpinner = spinner.Spinner{
		Frames: []string{"🕐 ", "🕑 ", "🕒 ", "🕓 ", "🕔 ", "🕕 ", "🕖 ", "🕗 ", "🕘 ", "🕙 ", "🕚 ", "🕛 "},
		FPS:    time.Second / 4,
	}

	foregroundBrightStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#111111", Dark: "#EEEEEE"}).Bold(true)
)

// confirmOptions shows a summary of the user's selections and prompts for confirmation.
func (r *Runner) confirmOptions(ctx context.Context, options *initOptions) (bool, error) {
	model := NewSummaryModel(*options)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	model, err := r.Prompter.RunProgram(program)
	if err != nil {
		return false, err
	}

	switch model.(*summaryModel).result {
	case resultConfimed:
		return true, nil
	case resultCanceled:
		return false, nil
	case resultQuit:
		return false, &prompt.ErrExitConsole{}
	default:
		panic("unknown result " + model.(*summaryModel).result)
	}
}

// showProgress shows an updating progress display while the user's selections are being applied.
//
// This function should be called from a goroutine while installation proceeds in the background.
// provide a channel to update progress.
func (r *Runner) showProgress(ctx context.Context, options *initOptions, progressChan <-chan progressMsg) error {
	model := NewProgessModel(*options)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	go func() {
		for msg := range progressChan {
			program.Send(msg)
		}

		program.Send(tea.Quit)
	}()

	_, err := r.Prompter.RunProgram(program)
	if err != nil {
		return err
	}

	return err
}

// progressMsg is a message sent to the progress display to update the status of the installation.
type progressMsg struct {
	InstallComplete     bool
	EnvironmentComplete bool
	ApplicationComplete bool
	ConfigComplete      bool
}

type summaryResult string

const (
	resultConfimed = "confirmed"
	resultCanceled = "canceled"
	resultQuit     = "quit"
)

var _ tea.Model = &summaryModel{}

type summaryModel struct {
	style   lipgloss.Style
	result  summaryResult
	options initOptions
}

// NewSummaryModel creates a new model for the options summary shown during 'rad init'.
func NewSummaryModel(options initOptions) tea.Model {
	return &summaryModel{
		style:   lipgloss.NewStyle().Margin(1, 0),
		options: options,
	}
}

// Init implements the init function for tea.Model. This will be called when the model is started, before View or
// Update are called.
func (m *summaryModel) Init() tea.Cmd {
	return nil
}

// Update implements the update function for tea.Model. This will be called when a message is received by the model.
//
// It's safe to update internal state inside this function. View will be called afterwards to draw the UI.
//
// # Function Explanation
//
// "summaryModel.Update" handles messages and state transitions, and returns the next model and command based on the type
// of message received. If the message is a KeyCtrlC, KeyEsc, or KeyEnter, the result is set accordingly and the command is
//
//	set to Quit. Otherwise, the message is ignored and no command is returned.
func (m *summaryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// This function handles messages and state transitions. We don't need to update
	// any UI here, just return the next model and command.
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			// User is quitting
			copy := *m
			copy.result = resultQuit
			return &copy, tea.Quit
		}
		if msg.Type == tea.KeyEsc {
			// User is canceling
			copy := *m
			copy.result = resultCanceled
			return &copy, tea.Quit
		}
		if msg.Type == tea.KeyEnter {
			// User has confirmed
			copy := *m
			copy.result = resultConfimed
			return &copy, tea.Quit // TODO: quit.
		}
	}

	// Ignore other messages
	return m, nil
}

// View implments the view function for tea.Model. This will be called after Init and after each call to Update to
// draw the UI.
func (m *summaryModel) View() string {
	// Hide when summary is dismissed
	if m.result != "" {
		return ""
	}

	options := m.options

	message := &strings.Builder{}
	message.WriteString(summaryHeading)

	message.WriteString(summaryKubernetesHeadingIcon)
	if options.Cluster.Install {
		message.WriteString(fmt.Sprintf(summaryKubernetesInstallHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context), highlight(options.Cluster.Namespace)))

		if options.CloudProviders.AWS != nil {
			message.WriteString(fmt.Sprintf(summaryKubernetesInstallAWSCloudProviderFmt, highlight(options.CloudProviders.AWS.AccessKeyID)))
		}
		if options.CloudProviders.Azure != nil {
			message.WriteString(fmt.Sprintf(summaryKubernetesInstallAzureCloudProviderFmt, highlight(options.CloudProviders.Azure.ServicePrincipal.ClientID)))
		}
	} else {
		message.WriteString(fmt.Sprintf(summaryKubernetesExistingHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context)))
	}

	message.WriteString(summaryEnvironmentHeadingIcon)
	if options.Environment.Create {
		message.WriteString(fmt.Sprintf(summaryEnvironmentCreateHeadingFmt, highlight(options.Environment.Name), highlight(options.Environment.Namespace)))

		if options.CloudProviders.AWS != nil {
			message.WriteString(fmt.Sprintf(summaryEnvironmentCreateAWSCloudProviderFmt, highlight(options.CloudProviders.AWS.AccountID), highlight(options.CloudProviders.AWS.Region)))
		}

		if options.CloudProviders.Azure != nil {
			message.WriteString(fmt.Sprintf(summaryEnvironmentCreateAzureCloudProviderFmt, highlight(options.CloudProviders.Azure.SubscriptionID), highlight(options.CloudProviders.Azure.ResourceGroup)))
		}

		if options.Recipes.DevRecipes {
			message.WriteString(fmt.Sprintf(summaryEnvironmentCreateRecipePackyFmt, highlight("dev")))
		}
	} else {
		message.WriteString(fmt.Sprintf(summaryEnvironmentExistingHeadingFmt, highlight(options.Environment.Name)))
	}

	if options.Application.Scaffold {
		message.WriteString(summaryApplicationHeadingIcon)
		message.WriteString(fmt.Sprintf(summaryApplicationScaffoldHeadingFmt, highlight(options.Application.Name)))
		message.WriteString(fmt.Sprintf(summaryApplicationScaffoldFile, highlight("app.bicep")))
		message.WriteString(fmt.Sprintf(summaryApplicationScaffoldFile, highlight(filepath.Join(".rad", "rad.yaml"))))
	}

	message.WriteString(summaryConfigurationHeadingIcon)
	message.WriteString(summaryConfigurationUpdateHeading)

	message.WriteString(summaryFooter)

	return m.style.Render(message.String())
}

var _ tea.Model = &progressModel{}

// NewProgessModel creates a new model for the initialization progress dialog shown during 'rad init'.
func NewProgessModel(options initOptions) tea.Model {
	return &progressModel{
		options: options,
		spinner: spinner.New(spinner.WithSpinner(progressSpinner)),

		// Setting a height here to avoid double-printing issues when the
		// hight of the output changes.
		style: lipgloss.NewStyle().Margin(1, 0),
	}
}

type progressModel struct {
	options  initOptions
	progress progressMsg
	spinner  spinner.Model
	style    lipgloss.Style

	// suppressSpinner is used to suppress the ticking of the spinner for testing.
	suppressSpinner bool
}

// Init implements the init function for tea.Model. This will be called when the model is started, before View or
// Update are called.
func (m *progressModel) Init() tea.Cmd {
	if m.suppressSpinner {
		return nil
	}

	// Start the spinner
	return m.spinner.Tick
}

// Update implements the update function for tea.Model. This will be called when a message is received by the model.
//
// It's safe to update internal state inside this function. View will be called afterwards to draw the UI.
//
// # Function Explanation
//
// Update updates the internal state of the progressModel when it receives a progressMsg or spinner.TickMsg,
// and returns a tea.Cmd to quit the program if the progress is complete.
func (m *progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// Update our internal state when we receive a progress update message.
	case progressMsg:
		m.progress = msg
		if m.isComplete() {
			return m, tea.Quit
		}

		return m, nil

	// Update spinner internal state when we receive a tick.
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

// View implments the view function for tea.Model. This will be called after Init and after each call to Update to
// draw the UI.
//
// # Function Explanation
//
// View builds a string containing a summary of the progress of a GO program, including the installation of
// Kubernetes, the creation of an environment, the scaffolding of an application, and the updating of configuration.
func (m *progressModel) View() string {
	options := m.options

	message := &strings.Builder{}
	message.WriteString(progressHeading)

	waiting := false // It's the hardest part.

	m.writeProgressIcon(message, m.progress.InstallComplete, &waiting)
	if options.Cluster.Install {
		message.WriteString(fmt.Sprintf(summaryKubernetesInstallHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context), highlight(options.Cluster.Namespace)))

		if options.CloudProviders.AWS != nil {
			message.WriteString(fmt.Sprintf(summaryKubernetesInstallAWSCloudProviderFmt, highlight(options.CloudProviders.AWS.AccessKeyID)))
		}
		if options.CloudProviders.Azure != nil {
			message.WriteString(fmt.Sprintf(summaryKubernetesInstallAzureCloudProviderFmt, highlight(options.CloudProviders.Azure.ServicePrincipal.ClientID)))
		}
	} else {
		message.WriteString(fmt.Sprintf(summaryKubernetesExistingHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context)))
	}

	m.writeProgressIcon(message, m.progress.EnvironmentComplete, &waiting)
	if options.Environment.Create {
		message.WriteString(fmt.Sprintf(summaryEnvironmentCreateHeadingFmt, highlight(options.Environment.Name), highlight(options.Environment.Namespace)))

		if options.CloudProviders.AWS != nil {
			message.WriteString(fmt.Sprintf(summaryEnvironmentCreateAWSCloudProviderFmt, highlight(options.CloudProviders.AWS.AccountID), highlight(options.CloudProviders.AWS.Region)))
		}

		if options.CloudProviders.Azure != nil {
			message.WriteString(fmt.Sprintf(summaryEnvironmentCreateAzureCloudProviderFmt, highlight(options.CloudProviders.Azure.SubscriptionID), highlight(options.CloudProviders.Azure.ResourceGroup)))
		}

		if options.Recipes.DevRecipes {
			message.WriteString(fmt.Sprintf(summaryEnvironmentCreateRecipePackyFmt, highlight("dev")))
		}
	} else {
		message.WriteString(fmt.Sprintf(summaryEnvironmentExistingHeadingFmt, highlight(options.Environment.Name)))
	}

	if options.Application.Scaffold {
		m.writeProgressIcon(message, m.progress.ApplicationComplete, &waiting)
		message.WriteString(fmt.Sprintf(summaryApplicationScaffoldHeadingFmt, highlight(options.Application.Name)))
	}

	m.writeProgressIcon(message, m.progress.ConfigComplete, &waiting)
	message.WriteString(summaryConfigurationUpdateHeading)

	if !waiting {
		// Everything is complete, so we're done.
		message.WriteString(progressCompleteFooter)
	}

	return m.style.Render(message.String())
}

func (m *progressModel) isComplete() bool {
	return m.progress.InstallComplete && m.progress.EnvironmentComplete && m.progress.ApplicationComplete && m.progress.ConfigComplete
}

// writeProgressIcon writes the correct icon for the progress step depending on the current step.
func (m *progressModel) writeProgressIcon(message *strings.Builder, condition bool, waiting *bool) {
	// Logic:
	//
	// - If the step is complete, write the complete icon.
	// - If we're waiting based on a previous step not being complete, write the waiting icon.
	// - If we're not waiting then this is the current step:
	//    - Show the spinner
	//    - Set waiting to true so that we show the waiting icon for the following steps.
	if condition {
		message.WriteString(progressStepCompleteIcon)
	} else if *waiting {
		message.WriteString(progressStepWaitingIcon)
	} else if m.suppressSpinner {
		// We can't render the *real* spinner without starting it, so just render a static glyph.
		message.WriteString(progressSpinner.Frames[0])
		*waiting = true
	} else {
		message.WriteString(m.spinner.View())
		*waiting = true
	}
}

func highlight(text string) string {
	return foregroundBrightStyle.Render(text)
}
