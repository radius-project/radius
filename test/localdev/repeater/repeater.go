/// 2>/dev/null; e=$(mktemp); go build -o $e "$0"; $e "$@" ; r=$?; rm $e; exit $r
// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Usage: repeater <timeout in seconds> <exit code>

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Printf("Expected 2 arguments but received %d\n", len(os.Args)-1)
		fmt.Printf("Usage: repeater <timeout in seconds> <exit code>\n")
		os.Exit(120)
	}

	timeout, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Printf("Invalid timeout value '%s'\n", os.Args[1])
		os.Exit(121)
	}

	exitCode, err := strconv.Atoi(os.Args[2])
	if err != nil || exitCode < 0 || exitCode > 119 {
		fmt.Printf("Invalid exit code value '%s'\n", os.Args[2])
		os.Exit(122)
	}

	time.AfterFunc(time.Duration(timeout)*time.Second, func() {
		os.Exit(exitCode)
	})

	buf := make([]byte, 120)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				select {}
			} else {
				fmt.Printf("Unexpected error while reading std input: %v\n", err)
				os.Exit(123)
			}
		}
	}
}
