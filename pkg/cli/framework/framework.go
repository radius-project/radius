// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package framework

import (
	"context"

	"github.com/project-radius/radius/pkg/cli/configFile"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
)

// Factory interface handles resources for interfacing with corerp and configs
type Factory interface {
	GetConnectionFactory() connections.Factory
	GetConfigHolder() *ConfigHolder
	GetOutput() output.Interface
	GetPrompter() prompt.Interface
	GetConfigFileInterface() configFile.Interface
}

type Impl struct {
	ConnectionFactory connections.Factory
	ConfigHolder      *ConfigHolder
	Output            output.Interface
	Prompter          prompt.Interface
	ConfigFileInterface configFile.Interface
}

func (i *Impl) GetConnectionFactory() connections.Factory {
	return i.ConnectionFactory
}

func (i *Impl) GetConfigHolder() *ConfigHolder {
	return i.ConfigHolder
}

func (i *Impl) GetOutput() output.Interface {
	return i.Output
}

func (i *Impl) GetPrompter() prompt.Interface {
	return i.Prompter
}

func (i *Impl) GetConfigFileInterface() configFile.Interface {
	return i.ConfigFileInterface
}

type Runner interface {
	Validate(cmd *cobra.Command, args []string) error
	Run(ctx context.Context) error
}

func RunCommand(runner Runner) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := runner.Validate(cmd, args)
		if err != nil {
			return err
		}

		err = runner.Run(cmd.Context())
		if err != nil {
			return err
		}

		return nil
	}
}
