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

// displayErrors displays errors (if any) in users' environment configuration when listing envs in List, Json, Table formats
func displayErrors(format string, cmd *cobra.Command, env cli.EnvironmentSection) (bool, error) {
	var (
		err           error
		errList       []envError
		errJsonList   []string
		isDefault     bool = false
		hasError      bool = false
		retenv             = envWithError{env, nil}
		defaultErrMsg      = "the default environment entry '%v' has not been configured"
	)

	for key := range env.Items {
		_, err := env.GetEnvironment(key)
		if err != nil {
			if format == output.FormatJson {
				errJsonList = append(errJsonList, err.Error())
			} else {
				errList = append(errList, envError{err.Error()})
			}
		}
		if env.Default == key {
			isDefault = true
		}
	}

	// check if default exists
	if env.Default != "" && !isDefault {
		if format == output.FormatJson {
			errJsonList = append(errJsonList, fmt.Sprintf(defaultErrMsg, env.Default))
		} else {
			errList = append(errList, envError{fmt.Sprintf(defaultErrMsg, env.Default)})
		}
	}

	if len(errJsonList) > 0 {
		hasError = true
		retenv.Errors = errJsonList
		err = output.Write(format, &retenv, cmd.OutOrStdout(), output.FormatterOptions{Columns: []output.Column{}})
		if err != nil {
			return hasError, err
		}
	} else if len(errList) > 0 {
		hasError = true
		fmt.Println()
		errformatter := objectformats.GetGenericEnvErrorTableFormat()
		err = output.Write(output.FormatTable, errList, cmd.OutOrStdout(), errformatter)
		if err != nil {
			return hasError, err
		}
	}

	return hasError, nil
}
