// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/project-radius/radius/pkg/tools/codegen/schema"
)

func checkFlags(inputFiles []string, outputFile string) {
	errors := []string{}
	if len(inputFiles) == 0 {
		errors = append(errors, "No input file provided.")
	}
	if outputFile == "" {
		errors = append(errors, "No output file provided. Please supply --output.")
	}
	if len(errors) == 0 {
		return
	}
	fmt.Println(strings.Join(errors, "\n"))
	fmt.Printf("Usage: %s --output foo.json input1.json input2.json ...", filepath.Base(os.Args[0]))
	os.Exit(-1)
}

func main() {
	outputFile := flag.String("output", "", "name of the output file")
	flag.Parse()
	inputFiles := flag.Args()

	checkFlags(inputFiles, *outputFile)

	// Read the input
	inputSchemas := make(map[string]schema.Schema, len(inputFiles))
	for _, inputFile := range inputFiles {
		s, err := schema.Load(inputFile)
		if err != nil {
			log.Fatalf("Error: cannot read file %q: %v", inputFile, err)
		}
		inputSchemas[inputFile] = *s
	}

	// Convert to use autorest's discriminator values.
	outputSchema, err := schema.NewAutorestConverter().Convert(inputSchemas)
	if err != nil {
		log.Fatalf("Error: fail to convert to autorest schema: %v", err)
	}

	// Write JSON to output file.
	b, _ := json.MarshalIndent(outputSchema, "", "  ")
	if err := ioutil.WriteFile(*outputFile, b, 0600); err != nil {
		log.Fatalf("Error: fail to write to file %s", *outputFile)
	}
}
