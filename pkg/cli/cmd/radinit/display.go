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
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/setup"
	"github.com/radius-project/radius/pkg/cli/style"
)

// Display constants used to render the summary and progress views shown by `rad init`.
const (
	summaryIndent                                 = "   - "
	summaryHeading                                = "You've selected the following:\n\n"
	summaryFooter                                 = "\n(press enter to confirm or esc to restart)\n"
	summaryKubernetesHeadingIcon                  = "🔧 "
	summaryKubernetesInstallHeadingFmt            = "Install Radius %s\n" + summaryIndent + "Kubernetes cluster: %s\n" + summaryIndent + "Kubernetes namespace: %s\n"
	summaryKubernetesInstallAWSCloudProviderFmt   = summaryIndent + "AWS credential: %s\n"
	summaryKubernetesInstallAzureCloudProviderFmt = summaryIndent + "Azure credential: %s\n"
	summaryKubernetesExistingHeadingFmt           = "Use existing Radius %s install on %s\n"
	summaryEnvironmentHeadingIcon                 = "🌏 "
	summaryEnvironmentCreateHeadingFmt            = "Create new environment %s\n" + summaryIndent + "Kubernetes namespace: %s\n"
	summaryEnvironmentCreateAWSCloudProviderFmt   = summaryIndent + "AWS: account %s and region %s\n"
	summaryEnvironmentCreateAzureCloudProviderFmt = summaryIndent + "Azure: subscription %s and resource group %s\n"
	summaryEnvironmentCreateRecipePackyFmt        = summaryIndent + "Recipe pack: %s\n"
	summaryEnvironmentExistingHeadingFmt          = "Use existing environment %s\n"
	summaryConfigurationHeadingIcon               = "📋 "
	summaryConfigurationUpdateHeading             = "Update local configuration\n"
	summaryConfigurationFile                      = summaryIndent + "%s\n"
	progressHeading                               = "Initializing Radius. This may take a minute or two...\n\n"
	progressCompleteFooter                        = "\nInitialization complete! Have a RAD time 😎\n\n"
	progressStepCompleteIcon                      = "✅ "
	progressStepWaitingIcon                       = "⏳ "
)

var (
	progressSpinner = spinner.Spinner{
		Frames: []string{"🕐 ", "🕑 ", "🕒 ", "🕓 ", "🕔 ", "🕕 ", "🕖 ", "🕗 ", "🕘 ", "🕙 ", "🕚 ", "🕛 "},
		FPS:    time.Second / 4,
	}

	foregroundBrightStyle = lipgloss.NewStyle().Foreground(style.AdaptiveColor{Light: lipgloss.Color("#111111"), Dark: lipgloss.Color("#EEEEEE")}).Bold(true)
)

// progressMsg is a message sent to the progress display to update the status of the installation.
type progressMsg struct {
	InstallComplete     bool
	EnvironmentComplete bool
	ConfigComplete      bool
}

// summaryResult represents the user's choice on the summary screen.
type summaryResult string

const (
	resultConfirmed summaryResult = "confirmed"
	resultCanceled  summaryResult = "canceled"
	resultQuit      summaryResult = "quit"
)

// confirmOptions shows a summary of the user's selections and prompts for confirmation.
func (r *Runner) confirmOptions(ctx context.Context, options *initOptions) (bool, error) {
	model := newSummaryModel(*options)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	model, err := r.Prompter.RunProgram(program)
	if err != nil {
		return false, err
	}

	switch model.(*summaryModel).Result {
	case resultConfirmed:
		return true, nil
	case resultCanceled:
		return false, nil
	case resultQuit:
		return false, &prompt.ErrExitConsole{}
	default:
		panic("unknown result " + model.(*summaryModel).Result)
	}
}

// redirectStdout redirects the process's stdout to the null device and returns the previous
// stdout writer along with a restore function. Callers render the progress UI to the returned
// writer so that stray writes to os.Stdout during installation (for example Helm's install
// logs) cannot corrupt the display. If the null device cannot be opened, stdout is left
// unchanged. The returned restore function is safe to call via defer.
func redirectStdout() (out io.Writer, restore func()) {
	real := os.Stdout
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return real, func() {}
	}

	os.Stdout = devNull
	return real, func() {
		os.Stdout = real
		_ = devNull.Close()
	}
}

// showProgress shows an updating progress display while the user's selections are being applied.
//
// This function should be called from a goroutine while installation proceeds in the background.
// Progress updates are received on progressChan; the loop also exits when ctx is canceled.
//
// The display is rendered to out rather than to os.Stdout directly. Callers redirect the
// process's stdout elsewhere while this UI runs so that stray writes (for example Helm's
// install logs) cannot corrupt the rendered output.
func (r *Runner) showProgress(ctx context.Context, options *initOptions, progressChan <-chan progressMsg, out io.Writer) error {
	model := newProgressModel(*options)
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithOutput(out))

	go func() {
		for {
			select {
			case <-ctx.Done():
				program.Quit()
				return
			case msg, ok := <-progressChan:
				if !ok {
					program.Quit()
					return
				}

				program.Send(msg)
			}
		}
	}()

	finalModel, err := r.Prompter.RunProgram(program)

	// If the user pressed Ctrl+C, abort rad init immediately. The installation steps run
	// in another goroutine and may be blocked in a call (such as a Helm install) that
	// cannot be canceled cooperatively, so terminate the process the way Helm does on
	// interrupt. The terminal has already been restored by Bubble Tea at this point.
	if pm, ok := finalModel.(*progressModel); ok && pm.Interrupted {
		os.Exit(130) //nolint:forbidigo // Intentional: match Helm's behavior and abort rad init on Ctrl+C.
	}

	return err
}

var _ tea.Model = &summaryModel{}

// summaryModel is the bubble tea model for the options summary shown during 'rad init'.
type summaryModel struct {
	style   lipgloss.Style
	Result  summaryResult
	Options initOptions
	width   int
}

// newSummaryModel creates a new model for the options summary shown during 'rad init'.
func newSummaryModel(options initOptions) tea.Model {
	return &summaryModel{
		style:   lipgloss.NewStyle().Margin(1, 0),
		Options: options,
	}
}

// Init implements tea.Model.
func (m *summaryModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. Pressing Ctrl+C quits, Esc cancels, and Enter confirms.
func (m *summaryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			copy := *m
			copy.Result = resultQuit
			return &copy, tea.Quit
		case "esc":
			copy := *m
			copy.Result = resultCanceled
			return &copy, tea.Quit
		case "enter":
			copy := *m
			copy.Result = resultConfirmed
			return &copy, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model. It renders the summary of selected options.
func (m *summaryModel) View() tea.View {
	if m.Result != "" {
		return tea.NewView("")
	}

	options := m.Options

	message := &strings.Builder{}
	message.WriteString(summaryHeading)

	message.WriteString(summaryKubernetesHeadingIcon)
	if options.Cluster.Install {
		message.WriteString(fmt.Sprintf(summaryKubernetesInstallHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context), highlight(options.Cluster.Namespace)))
		writeCloudProviderInstallSummary(message, options.CloudProviders)
	} else {
		message.WriteString(fmt.Sprintf(summaryKubernetesExistingHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context)))
	}

	message.WriteString(summaryEnvironmentHeadingIcon)
	writeEnvironmentSummary(message, options)

	message.WriteString(summaryConfigurationHeadingIcon)
	message.WriteString(summaryConfigurationUpdateHeading)
	writeConfigFiles(message, options)

	message.WriteString(summaryFooter)

	return tea.NewView(m.style.Render(ansi.Hardwrap(message.String(), m.width, true)))
}

var _ tea.Model = &progressModel{}

// newProgressModel creates a new model for the initialization progress dialog shown during 'rad init'.
func newProgressModel(options initOptions) tea.Model {
	return &progressModel{
		Options: options,
		spinner: spinner.New(spinner.WithSpinner(progressSpinner)),

		// Setting a height here to avoid double-printing issues when the
		// height of the output changes.
		style: lipgloss.NewStyle().Margin(1, 0),
	}
}

// progressModel is the bubble tea model for the progress display shown during 'rad init'.
type progressModel struct {
	Options  initOptions
	Progress progressMsg
	spinner  spinner.Model
	style    lipgloss.Style

	// Interrupted is set when the user cancels the progress display with Ctrl+C.
	Interrupted bool

	// SuppressSpinner is used to suppress the ticking of the spinner for testing.
	SuppressSpinner bool
	width           int
}

// Init implements tea.Model.
func (m *progressModel) Init() tea.Cmd {
	if m.SuppressSpinner {
		return nil
	}

	return m.spinner.Tick
}

// Update implements tea.Model. It updates the model state on progress updates and spinner ticks.
func (m *progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.Interrupted = true
			return m, tea.Quit
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case progressMsg:
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
func (m *progressModel) View() tea.View {
	options := m.Options

	message := &strings.Builder{}
	message.WriteString(progressHeading)

	waiting := false

	m.writeProgressIcon(message, m.Progress.InstallComplete, &waiting)
	if options.Cluster.Install {
		message.WriteString(fmt.Sprintf(summaryKubernetesInstallHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context), highlight(options.Cluster.Namespace)))
		writeCloudProviderInstallSummary(message, options.CloudProviders)
	} else {
		message.WriteString(fmt.Sprintf(summaryKubernetesExistingHeadingFmt, highlight(options.Cluster.Version), highlight(options.Cluster.Context)))
	}

	m.writeProgressIcon(message, m.Progress.EnvironmentComplete, &waiting)
	writeEnvironmentSummary(message, options)

	m.writeProgressIcon(message, m.Progress.ConfigComplete, &waiting)
	message.WriteString(summaryConfigurationUpdateHeading)
	writeConfigFiles(message, options)

	if !waiting {
		message.WriteString(progressCompleteFooter)
	}

	return tea.NewView(m.style.Render(ansi.Hardwrap(message.String(), m.width, true)))
}

func (m *progressModel) isComplete() bool {
	return m.Progress.InstallComplete && m.Progress.EnvironmentComplete && m.Progress.ConfigComplete
}

// writeProgressIcon writes the correct icon for the progress step depending on the current step.
//
// Logic:
//   - If the step is complete, write the complete icon.
//   - If we're waiting based on a previous step not being complete, write the waiting icon.
//   - If we're not waiting then this is the current step:
//   - Show the spinner.
//   - Set waiting to true so that we show the waiting icon for the following steps.
func (m *progressModel) writeProgressIcon(message *strings.Builder, condition bool, waiting *bool) {
	if condition {
		message.WriteString(progressStepCompleteIcon)
	} else if *waiting {
		message.WriteString(progressStepWaitingIcon)
	} else if m.SuppressSpinner {
		// We can't render the *real* spinner without starting it, so just render a static glyph.
		message.WriteString(progressSpinner.Frames[0])
		*waiting = true
	} else {
		message.WriteString(m.spinner.View())
		*waiting = true
	}
}

func writeCloudProviderInstallSummary(message *strings.Builder, providers cloudProviderOptions) {
	if providers.AWS != nil {
		message.WriteString(fmt.Sprintf(summaryKubernetesInstallAWSCloudProviderFmt, highlight(string(providers.AWS.CredentialKind))))
		switch providers.AWS.CredentialKind {
		case aws.AWSCredentialKindAccessKey:
			message.WriteString(fmt.Sprintf(summaryIndent+"AccessKey ID: %s\n", highlight(providers.AWS.AccessKey.AccessKeyID)))
		case aws.AWSCredentialKindIRSA:
			message.WriteString(fmt.Sprintf(summaryIndent+"IAM Role ARN: %s\n", highlight(providers.AWS.IRSA.RoleARN)))
		}
	}
	if providers.Azure != nil {
		message.WriteString(fmt.Sprintf(summaryKubernetesInstallAzureCloudProviderFmt, highlight(string(providers.Azure.CredentialKind))))
		switch providers.Azure.CredentialKind {
		case azure.AzureCredentialKindServicePrincipal:
			message.WriteString(fmt.Sprintf(summaryIndent+"Client ID: %s\n", highlight(providers.Azure.ServicePrincipal.ClientID)))
		case azure.AzureCredentialKindWorkloadIdentity:
			message.WriteString(fmt.Sprintf(summaryIndent+"Client ID: %s\n", highlight(providers.Azure.WorkloadIdentity.ClientID)))
		}
	}
}

func writeEnvironmentSummary(message *strings.Builder, options initOptions) {
	if options.Environment.Create {
		message.WriteString(fmt.Sprintf(summaryEnvironmentCreateHeadingFmt, highlight(options.Environment.Name), highlight(options.Environment.Namespace)))

		if options.CloudProviders.AWS != nil {
			message.WriteString(fmt.Sprintf(summaryEnvironmentCreateAWSCloudProviderFmt, highlight(options.CloudProviders.AWS.AccountID), highlight(options.CloudProviders.AWS.Region)))
		}

		if options.CloudProviders.Azure != nil {
			message.WriteString(fmt.Sprintf(summaryEnvironmentCreateAzureCloudProviderFmt, highlight(options.CloudProviders.Azure.SubscriptionID), highlight(options.CloudProviders.Azure.ResourceGroup)))
		}

		if options.Recipes.DefaultRecipePack {
			message.WriteString(fmt.Sprintf(summaryEnvironmentCreateRecipePackyFmt, highlight("default")))
		}
	} else {
		message.WriteString(fmt.Sprintf(summaryEnvironmentExistingHeadingFmt, highlight(options.Environment.Name)))
	}
}

// writeConfigFiles writes one indented line per local configuration file that rad init writes.
func writeConfigFiles(message *strings.Builder, options initOptions) {
	if options.BicepConfigDirectory == "" {
		return
	}

	message.WriteString(fmt.Sprintf(summaryConfigurationFile, highlight(filepath.Join(options.BicepConfigDirectory, setup.BicepConfigFileName))))
}

func highlight(text string) string {
	return foregroundBrightStyle.Render(text)
}
