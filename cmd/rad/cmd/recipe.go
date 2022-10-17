// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

func NewRecipeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "recipe",
		Short: "Manage connector recipes",
		Long: `Manage connector recipes
		Connector recipes automate the deployment of infrastructure and configuration of connectors.`,
	}
}

func init() {
	RootCmd.AddCommand(recipeCmd)
	recipeCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
	recipeCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}
