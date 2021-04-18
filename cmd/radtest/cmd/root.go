// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "radtest [directory]",
	Short: "Project Radius radtest CLI",
	Long: `Project Radius radtest CLI
	
radtest is a "close enough for jazz" test CLI for the Radius RP.

Use radtest to compile and deploy .bicep files against a local copy of the Radius RP - no cloud provided needed!`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// Validate args and find the bicep file to use.
func validate(args []string) (string, error) {
	// Use current directory by default
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Use directory if one was provided
	if len(args) == 1 {
		f, err := os.Stat(args[0])
		if err != nil {
			return "", err
		}

		if !f.IsDir() {
			// User passed in a file, use it.
			return args[0], nil
		}

		dir = args[0]
	}

	// if we have a directory, verify that we have a single .bicep file
	matches, err := filepath.Glob(filepath.Join(dir, "/*.bicep"))
	if err != nil {
		return "", err
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no .bicep files found in directory '%s'", dir)
	} else if len(matches) > 1 {
		return "", fmt.Errorf("multiple .bicep files were found in directory '%s'. specify the desired filename", dir)
	}

	return matches[0], nil
}
