// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/radius-project/radius/pkg/ucp/authorization/internal/cataloggen"
)

func main() {
	root := flag.String("root", ".", "repository root")
	output := flag.String("output", "pkg/ucp/authorization/catalog_generated.json", "output file")
	flag.Parse()
	if err := run(*root, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) //nolint:forbidigo // Command entry point.
	}
}

func run(root, output string) error {
	catalog, err := cataloggen.Generate(root)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(output, append(data, '\n'), 0644)
}
