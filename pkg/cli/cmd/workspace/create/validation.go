// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ValidateArgs returns an error if the args .
func ValidateArgs() cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return fmt.Errorf("usage: rad workspace create [workspaceType] [workspaceName] [flags]")
		}
		if args[0] != "kubernetes" {
			return fmt.Errorf("workspaces currently only support type 'kubernetes'")
		}
		return nil
	}
}
