// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"fmt"

	"github.com/Azure/radius/cmd/cli/cmd"
)

func main() {
	fmt.Println("someone new was here")
	cmd.Execute()
}
