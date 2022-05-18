// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/cobra"
)

type envError struct {
	Errors string
}

type envWithError struct {
	cli.EnvironmentSection
	Errors []string `json:"errors"`
}

// populateEnvErrors populates errors (if any) in users' environment configuration and is used when listing envs in JSON format
func populateEnvErrors(env cli.EnvironmentSection) interface{} {
	var (
		retenv    = envWithError{env, nil}
		errList   []string
		isDefault bool = false
	)

	for key := range env.Items {
		_, err := env.GetEnvironment(key)
		if err != nil {
			errList = append(errList, err.Error())
		}

		if env.Default == key {
			isDefault = true
		}

	}

	if env.Default != "" && !isDefault {
		errList = append(errList, fmt.Sprintf("the default environment entry %v has not been configured", env.Default))
	}
	if len(errList) > 0 {
		retenv.Errors = errList
		return retenv
	}

	return env
}

// displayErrors displays errors (if any) in users' environment configuration when listing envs in List and Table formats
func displayErrors(cmd *cobra.Command, env cli.EnvironmentSection) error {

	var (
		errList   []envError
		isDefault bool = false
		err       error
	)

	for key := range env.Items {
		_, err := env.GetEnvironment(key)
		if err != nil {
			errList = append(errList, envError{err.Error()})
		}
		if env.Default == key {
			isDefault = true
		}
	}

	// check if default exists
	if env.Default != "" && !isDefault {
		errList = append(errList, envError{fmt.Sprintf("the default environment entry %v has not been configured", env.Default)})
	}
	if len(errList) > 0 {
		fmt.Println()
		errformatter := objectformats.GetGenericEnvErrorTableFormat()
		err = output.Write(output.FormatTable, errList, cmd.OutOrStdout(), errformatter)
		if err != nil {
			return err
		}
	}
	return nil
}
