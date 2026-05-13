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

package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/prompt"
)

// Display constants used to render the summary and progress views shown by
// `rad init` (and its preview variant).
const (
	SummaryIndent                                 = "   - "
	SummaryHeading                                = "You've selected the following:\n\n"
	SummaryFooter                                 = "\n(press enter to confirm or esc to restart)\n"
	SummaryKubernetesHeadingIcon                  = "🔧 "
	SummaryKubernetesInstallHeadingFmt            = "Install Radius %s\n" + SummaryIndent + "Kubernetes cluster: %s\n" + SummaryIndent + "Kubernetes namespace: %s\n"
	SummaryKubernetesInstallAWSCloudProviderFmt   = SummaryIndent + "AWS credential: %s\n"
	SummaryKubernetesInstallAzureCloudProviderFmt = SummaryIndent + "Azure credential: %s\n"
	SummaryKubernetesExistingHeadingFmt           = "Use existing Radius %s install on %s\n"
	SummaryEnvironmentHeadingIcon                 = "🌏 "
	SummaryEnvironmentCreateHeadingFmt            = "Create new environment %s\n" + SummaryIndent + "Kubernetes namespace: %s\n"
	SummaryEnvironmentCreateAWSCloudProviderFmt   = SummaryIndent + "AWS: account %s and region %s\n"
	SummaryEnvironmentCreateAzureCloudProviderFmt = SummaryIndent + "Azure: subscription %s and resource group %s\n"
	SummaryEnvironmentCreateRecipePackyFmt        = SummaryIndent + "Recipe pack: %s\n"
	SummaryEnvironmentExistingHeadingFmt          = "Use existing environment %s\n"
	SummaryApplicationHeadingIcon                 = "🚧 "
	SummaryApplicationScaffoldHeadingFmt          = "Scaffold application %s\n"
	SummaryApplicationScaffoldFile                = SummaryIndent + "Create %s\n"
	SummaryConfigurationHeadingIcon               = "📋 "
	SummaryConfigurationUpdateHeading             = "Update local configuration\n"
	ProgressHeading                               = "Initializing Radius. This may take a minute or two...\n\n"
	ProgressCompleteFooter                        = "\nInitialization complete! Have a RAD time 😎\n\n"
	ProgressStepCompleteIcon                      = "✅ "
	ProgressStepWaitingIcon                       = "⏳ "
)

var (
	progressSpinner = spinner.Spinner{
		Frames: []string{"🕐 ", "🕑 ", "🕒 ", "🕓 ", "🕔 ", "🕕 ", "🕖 ", "🕗 ", "🕘 ", "🕙 ", "🕚 ", "🕛 "},
		FPS:    time.Second / 4,
	}

	foregroundBrightStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#111111", Dark: "#EEEEEE"}).Bold(true)
)

// DisplayOptions is the data model rendered by the summary and progress views.
//
// Callers convert their package-specific options struct into a DisplayOptions
// before invoking ConfirmOptions or ShowProgress.
type DisplayOptions struct {
	Cluster        ClusterDisplay
	Environment    EnvironmentDisplay
	CloudProviders CloudProvidersDisplay
	Application    ApplicationDisplay

	// RecipePackLabel is the label of the recipe pack to display in the summary.
	// An empty value omits the recipe pack line entirely.
	RecipePackLabel string
}

// ClusterDisplay holds the cluster fields rendered by the summary and progress views.
type ClusterDisplay struct {
	Install   bool
	Namespace string
	Context   string
	Version   string
}

// EnvironmentDisplay holds the environment fields rendered by the summary and progress views.
type EnvironmentDisplay struct {
	Create    bool
	Name      string
	Namespace string
}

// CloudProvidersDisplay holds the cloud provider fields rendered by the summary and progress views.
type CloudProvidersDisplay struct {
	Azure *azure.Provider
	AWS   *aws.Provider
}

// ApplicationDisplay holds the application fields rendered by the summary and progress views.
type ApplicationDisplay struct {
	Scaffold bool
	Name     string
	// ScaffoldFiles are the files to list under the scaffold application heading.
	ScaffoldFiles []string
}

// ProgressMsg is a message sent to the progress display to update the status of the installation.
type ProgressMsg struct {
	InstallComplete     bool
	EnvironmentComplete bool
	ApplicationComplete bool
	ConfigComplete      bool
}

// SummaryResult represents the user's choice on the summary screen.
type SummaryResult string

const (
	ResultConfirmed SummaryResult = "confirmed"
	ResultCanceled  SummaryResult = "canceled"
	ResultQuit      SummaryResult = "quit"
)

// ConfirmOptions shows a summary of the user's selections and prompts for confirmation.
func ConfirmOptions(ctx context.Context, prompter prompt.Interface, options DisplayOptions) (bool, error) {
	model := NewSummaryModel(options)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	model, err := prompter.RunProgram(program)
	if err != nil {
		return false, err
	}

	switch model.(*SummaryModel).Result {
	case ResultConfirmed:
		return true, nil
	case ResultCanceled:
		return false, nil
	case ResultQuit:
		return false, &prompt.ErrExitConsole{}
	default:
		panic("unknown result " + model.(*SummaryModel).Result)
	}
}

// ShowProgress shows an updating progress display while the user's selections are being applied.
//
// This function should be called from a goroutine while installation proceeds in the background.
// Progress updates are received on progressChan; the loop also exits when ctx is canceled.
func ShowProgress(ctx context.Context, prompter prompt.Interface, options DisplayOptions, progressChan <-chan ProgressMsg) error {
	model := NewProgressModel(options)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	go func() {
		for {
			select {
			case <-ctx.Done():
				program.Send(tea.Quit)
				return
			case msg, ok := <-progressChan:
				if !ok {
					program.Send(tea.Quit)
					return
				}

				program.Send(msg)
			}
		}
	}()

	_, err := prompter.RunProgram(program)
	return err
}

var _ tea.Model = &SummaryModel{}

// SummaryModel is the bubble tea model for the options summary shown during 'rad init'.
type SummaryModel struct {
	style   lipgloss.Style
	Result  SummaryResult
	Options DisplayOptions
	width   int
}

// NewSummaryModel creates a new model for the options summary shown during 'rad init'.
func NewSummaryModel(options DisplayOptions) tea.Model {
	return &SummaryModel{
		style:   lipgloss.NewStyle().Margin(1, 0),
		Options: options,
	}
}

// Init implements tea.Model.
func (m *SummaryModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. Pressing Ctrl+C quits, Esc cancels, and Enter confirms.
func (m *SummaryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			copy := *m
			copy.Result = ResultQuit
			return &copy, tea.Quit
		}
		if msg.Type == tea.KeyEsc {
			copy := *m
			copy.Result = ResultCanceled
			return &copy, tea.Quit
		}
		if msg.Type == tea.KeyEnter {
			copy := *m
			copy.Result = ResultConfirmed
			return &copy, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model. It renders the summary of selected options.
func (m *SummaryModel) View() string {
	if m.Result != "" {
		return ""
	}

	options := m.Options

	message := &strings.Builder{}
	message.WriteString(SummaryHeading)

	message.WriteString(SummaryKubernetesHeadingIcon)
	if options.Cluster.Install {
		message.WriteString(fmt.Sprintf(SummaryKubernetesInstallHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context), highlight(options.Cluster.Namespace)))
		writeCloudProviderInstallSummary(message, options.CloudProviders)
	} else {
		message.WriteString(fmt.Sprintf(SummaryKubernetesExistingHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context)))
	}

	message.WriteString(SummaryEnvironmentHeadingIcon)
	writeEnvironmentSummary(message, options)

	if options.Application.Scaffold {
		message.WriteString(SummaryApplicationHeadingIcon)
		message.WriteString(fmt.Sprintf(SummaryApplicationScaffoldHeadingFmt, highlight(options.Application.Name)))
		for _, file := range options.Application.ScaffoldFiles {
			message.WriteString(fmt.Sprintf(SummaryApplicationScaffoldFile, highlight(file)))
		}
	}

	message.WriteString(SummaryConfigurationHeadingIcon)
	message.WriteString(SummaryConfigurationUpdateHeading)

	message.WriteString(SummaryFooter)

	return m.style.Render(ansi.Hardwrap(message.String(), m.width, true))
}

var _ tea.Model = &ProgressModel{}

// NewProgressModel creates a new model for the initialization progress dialog shown during 'rad init'.
func NewProgressModel(options DisplayOptions) tea.Model {
	return &ProgressModel{
		Options: options,
		spinner: spinner.New(spinner.WithSpinner(progressSpinner)),

		// Setting a height here to avoid double-printing issues when the
		// height of the output changes.
		style: lipgloss.NewStyle().Margin(1, 0),
	}
}

// ProgressModel is the bubble tea model for the progress display shown during 'rad init'.
type ProgressModel struct {
	Options  DisplayOptions
	Progress ProgressMsg
	spinner  spinner.Model
	style    lipgloss.Style

	// SuppressSpinner is used to suppress the ticking of the spinner for testing.
	SuppressSpinner bool
	width           int
}

// Init implements tea.Model.
func (m *ProgressModel) Init() tea.Cmd {
	if m.SuppressSpinner {
		return nil
	}

	return m.spinner.Tick
}

// Update implements tea.Model. It updates the model state on progress updates and spinner ticks.
func (m *ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case ProgressMsg:
		m.Progress = msg
		if m.isComplete() {
			return m, tea.Quit
		}

		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

// View implements tea.Model. It renders the progress of the initialization steps.
func (m *ProgressModel) View() string {
	options := m.Options

	message := &strings.Builder{}
	message.WriteString(ProgressHeading)

	waiting := false

	m.writeProgressIcon(message, m.Progress.InstallComplete, &waiting)
	if options.Cluster.Install {
		message.WriteString(fmt.Sprintf(SummaryKubernetesInstallHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context), highlight(options.Cluster.Namespace)))
		writeCloudProviderInstallSummary(message, options.CloudProviders)
	} else {
		message.WriteString(fmt.Sprintf(SummaryKubernetesExistingHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context)))
	}

	m.writeProgressIcon(message, m.Progress.EnvironmentComplete, &waiting)
	writeEnvironmentSummary(message, options)

	if options.Application.Scaffold {
		m.writeProgressIcon(message, m.Progress.ApplicationComplete, &waiting)
		message.WriteString(fmt.Sprintf(SummaryApplicationScaffoldHeadingFmt, highlight(options.Application.Name)))
	}

	m.writeProgressIcon(message, m.Progress.ConfigComplete, &waiting)
	message.WriteString(SummaryConfigurationUpdateHeading)

	if !waiting {
		message.WriteString(ProgressCompleteFooter)
	}

	return m.style.Render(ansi.Hardwrap(message.String(), m.width, true))
}

func (m *ProgressModel) isComplete() bool {
	return m.Progress.InstallComplete && m.Progress.EnvironmentComplete && m.Progress.ApplicationComplete && m.Progress.ConfigComplete
}

// writeProgressIcon writes the correct icon for the progress step depending on the current step.
//
// Logic:
//   - If the step is complete, write the complete icon.
//   - If we're waiting based on a previous step not being complete, write the waiting icon.
//   - If we're not waiting then this is the current step:
//   - Show the spinner.
//   - Set waiting to true so that we show the waiting icon for the following steps.
func (m *ProgressModel) writeProgressIcon(message *strings.Builder, condition bool, waiting *bool) {
	if condition {
		message.WriteString(ProgressStepCompleteIcon)
	} else if *waiting {
		message.WriteString(ProgressStepWaitingIcon)
	} else if m.SuppressSpinner {
		// We can't render the *real* spinner without starting it, so just render a static glyph.
		message.WriteString(progressSpinner.Frames[0])
		*waiting = true
	} else {
		message.WriteString(m.spinner.View())
		*waiting = true
	}
}

func writeCloudProviderInstallSummary(message *strings.Builder, providers CloudProvidersDisplay) {
	if providers.AWS != nil {
		message.WriteString(fmt.Sprintf(SummaryKubernetesInstallAWSCloudProviderFmt, highlight(string(providers.AWS.CredentialKind))))
		switch providers.AWS.CredentialKind {
		case aws.AWSCredentialKindAccessKey:
			message.WriteString(fmt.Sprintf(SummaryIndent+"AccessKey ID: %s\n", highlight(providers.AWS.AccessKey.AccessKeyID)))
		case aws.AWSCredentialKindIRSA:
			message.WriteString(fmt.Sprintf(SummaryIndent+"IAM Role ARN: %s\n", highlight(providers.AWS.IRSA.RoleARN)))
		}
	}
	if providers.Azure != nil {
		message.WriteString(fmt.Sprintf(SummaryKubernetesInstallAzureCloudProviderFmt, highlight(string(providers.Azure.CredentialKind))))
		switch providers.Azure.CredentialKind {
		case azure.AzureCredentialKindServicePrincipal:
			message.WriteString(fmt.Sprintf(SummaryIndent+"Client ID: %s\n", highlight(providers.Azure.ServicePrincipal.ClientID)))
		case azure.AzureCredentialKindWorkloadIdentity:
			message.WriteString(fmt.Sprintf(SummaryIndent+"Client ID: %s\n", highlight(providers.Azure.WorkloadIdentity.ClientID)))
		}
	}
}

func writeEnvironmentSummary(message *strings.Builder, options DisplayOptions) {
	if options.Environment.Create {
		message.WriteString(fmt.Sprintf(SummaryEnvironmentCreateHeadingFmt, highlight(options.Environment.Name), highlight(options.Environment.Namespace)))

		if options.CloudProviders.AWS != nil {
			message.WriteString(fmt.Sprintf(SummaryEnvironmentCreateAWSCloudProviderFmt, highlight(options.CloudProviders.AWS.AccountID), highlight(options.CloudProviders.AWS.Region)))
		}

		if options.CloudProviders.Azure != nil {
			message.WriteString(fmt.Sprintf(SummaryEnvironmentCreateAzureCloudProviderFmt, highlight(options.CloudProviders.Azure.SubscriptionID), highlight(options.CloudProviders.Azure.ResourceGroup)))
		}

		if options.RecipePackLabel != "" {
			message.WriteString(fmt.Sprintf(SummaryEnvironmentCreateRecipePackyFmt, highlight(options.RecipePackLabel)))
		}
	} else {
		message.WriteString(fmt.Sprintf(SummaryEnvironmentExistingHeadingFmt, highlight(options.Environment.Name)))
	}
}

func highlight(text string) string {
	return foregroundBrightStyle.Render(text)
}
