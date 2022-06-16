// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package swagger

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"strings"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
)

var (
	// The listing of files below has an ordering to them, because
	// each file may depend on one or more files on the preceding
	// lines.

	//go:embed specification/applications/resource-manager/Applications.Connector/preview/2022-03-15-privatepreview/*.json
	//go:embed specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/*.json
	//go:embed specification/common-types/resource-management/v2/types.json
	schemaFiles embed.FS

	validators map[string]validator = loadOrPanic()
)

func HasType(resourceType string) bool {
	// ARM types are compared case-insensitively
	_, ok := validators[strings.ToLower(resourceType)]
	return ok
}

func GetValidator(resourceType string) (Validator, bool) {
	// ARM types are compared case-insensitively
	validator, ok := validators[strings.ToLower(resourceType)]
	if ok {
		return &validator, true
	}

	return nil, false
}

func loadOrPanic() map[string]validator {
	err := fs.WalkDir(schemaFiles, ".", func(path string, d fs.DirEntry, _ error) error {
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(schemaFiles, path)
		if err != nil {
			return fmt.Errorf("cannot read embedded file %s: %w", path, err)
		}

		// Load OpenAPI Spec
		specDoc, err := loads.JSONSpec("../../swagger/specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/environments.json")
		if err != nil {
			panic(err)
		}
		// Expand external references.
		wDoc, err := specDoc.Expanded(&spec.ExpandOptions{
			RelativeBase: "../../swagger/specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/environments.json",
		})

		return nil
	})
	if err != nil {
		log.Fatal("Failed to load schemas:", err)
	}
	validators := map[string]validator{}

	/*
		for resourceType, ref := range ResourceManifest.Resources {

			// The default logic of the schema loader for references is pretty obtuse. If you give
			// it a reference then it can load from the pool, this is what we want. None of the built-in
			// loaders have this behavior.
			//
			// - Loading from a string will 'poison' the cache because the schema doesn't have a unique reference
			// - Other loaders hit the filesystem/internet which we DO NOT WANT for security reasons.
			workaround := &StrictReferenceLoader{
				Reference: ref,
			}
			schema, err := loader.Compile(workaround)
			if err != nil {
				log.Fatalf("Failed to parse JSON Schema %q: %s", ref, err)
			}

			// ARM types are compared case-insensitively
			validators[strings.ToLower(resourceType)] = validator{
				schema:   schema,
				TypeName: resourceType,
			}
		}*/

	return validators
}
