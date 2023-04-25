// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"os"

	"github.com/project-radius/radius/cmd/rad/cmd"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		os.Exit(1) //nolint:forbidigo // this is OK inside the main function.
	}
}
