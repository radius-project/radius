// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"github.com/spf13/cobra"
)

var recipeCmd = &cobra.Command{
	Use:   "recipe",
	Short: "Manage recipes",
	Long:  `Manage recipes`,
}

func init() {
	RootCmd.AddCommand(recipeCmd)
	recipeCmd.PersistentFlags().StringP("environment", "e", "", "The environment name")
	recipeCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}
