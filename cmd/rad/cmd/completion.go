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

package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var completionExample = `
	# Installing bash completion on macOS using homebrew
	## If running Bash 3.2 included with macOS
	brew install bash-completion
	## or, if running Bash 4.1+
	brew install bash-completion@2
	## Add the completion to your completion directory
	rad completion bash > $(brew --prefix)/etc/bash_completion.d/rad
	source ~/.bash_profile
	# Installing bash completion on Linux
	## If bash-completion is not installed on Linux, please install the 'bash-completion' package
	## via your distribution's package manager.
	## Load the rad completion code for bash into the current shell
	source <(rad completion bash)
	## Write bash completion code to a file and source if from .bash_profile
	rad completion bash > ~/.rad/completion.bash.inc
	printf "
	## rad shell completion
	source '$HOME/.rad/completion.bash.inc'
	" >> $HOME/.bash_profile
	source $HOME/.bash_profile
	# Installing zsh completion on macOS using homebrew
	## If zsh-completion is not installed on macOS, please install the 'zsh-completion' package
	brew install zsh-completions
	## Set the rad completion code for zsh[1] to autoload on startup
	rad completion zsh > "${fpath[1]}/_rad"
	source ~/.zshrc
	# Installing zsh completion on Linux
	## If zsh-completion is not installed on Linux, please install the 'zsh-completion' package
	## via your distribution's package manager.
	## Load the rad completion code for zsh into the current shell
	source <(rad completion zsh)
	# Set the rad completion code for zsh[1] to autoload on startup
	rad completion zsh > "${fpath[1]}/_rad"
	# Installing powershell completion on Windows
	## Create $PROFILE if it not exists
	if (!(Test-Path -Path $PROFILE )){ New-Item -Type File -Path $PROFILE -Force }
	## Add the completion to your profile
	rad completion powershell >> $PROFILE
`

var completionCommand = &cobra.Command{
	Use:     "completion",
	Short:   "Generates shell completion scripts",
	Example: completionExample,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var completionZshCommand = &cobra.Command{
	Use:   "zsh",
	Short: "Generates zsh completion scripts",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RootCmd.GenZshCompletion(os.Stdout)
	},
}

var completionBashCommand = &cobra.Command{
	Use:   "bash",
	Short: "Generates bash completion scripts",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RootCmd.GenBashCompletion(os.Stdout)
	},
}

var completionPowershellCommand = &cobra.Command{
	Use:   "powershell",
	Short: "Generates powershell completion scripts",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RootCmd.GenPowerShellCompletion(os.Stdout)
	},
}

func init() {
	RootCmd.AddCommand(completionCommand)
	completionCommand.AddCommand(completionZshCommand)
	completionCommand.AddCommand(completionBashCommand)
	completionCommand.AddCommand(completionPowershellCommand)
}
