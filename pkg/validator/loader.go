// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"encoding/json"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"sync"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
)

var (
	ErrSpecDocumentNotFound = errors.New("not found OpenAPI specification document")
	ErrUndefinedAPI         = errors.New("undefined API")
)

// NewLoader creates OpenAPI spec loader.
func NewLoader(providerName string, specs fs.FS) *Loader {
	return &Loader{
		providerName: providerName,
		validators:   map[string]validator{},
		specFiles:    specs,
	}
}

// Loader is the OpenAPI spec loader implementation.
type Loader struct {
	validators   map[string]validator
	providerName string
	specFiles    fs.FS
}

// Name returns the name of loader.
func (l *Loader) Name() string {
	return l.providerName
}

// GetValidator returns the cached validator.
func (l *Loader) GetValidator(resourceType, version string) (Validator, bool) {
	// ARM types are compared case-insensitively
	validator, ok := l.validators[getValidatorKey(resourceType, version)]
	if ok {
		return &validator, true
	}
	return nil, false
}

// LoadSpec loads the swagger files and caches the validator.
func (l *Loader) LoadSpec() error {
	// Walk through embedded files to load OpenAPI spec document.
	err := fs.WalkDir(l.specFiles, ".", func(path string, d fs.DirEntry, _ error) error {
		if d.IsDir() {
			return nil
		}

		// Skip the shared common-types
		if strings.HasPrefix(path, "specification/common-types") {
			return nil
		}

		// Check if specification file pathname is valid and skip global.json.
		parsed := parseSpecFilePath(path)
		if pn, ok := parsed["provider"]; !ok || !strings.EqualFold(pn, l.providerName) || parsed["resourcetype"] == "global" {
			return nil
		}

		// Load OpenAPI Spec
		specDoc, err := loads.Spec(
			path,
			loads.WithDocLoader(func(path string) (json.RawMessage, error) {
				data, err := fs.ReadFile(l.specFiles, path)
				return json.RawMessage(data), err
			}))
		if err != nil {
			return err
		}

		// Expand $ref external references.
		wDoc, err := specDoc.Expanded(&spec.ExpandOptions{
			RelativeBase: path,
			PathLoader: func(path string) (json.RawMessage, error) {
				// Trim before 'specification' to convert relative path.
				first := strings.Index(path, "specification")
				data, err := fs.ReadFile(l.specFiles, path[first:])
				return json.RawMessage(data), err
			},
		})
		if err != nil {
			return err
		}

		key := getValidatorKey(parsed["resourcetype"], parsed["version"])
		l.validators[key] = validator{
			TypeName:     parsed["provider"] + "/" + parsed["resourcetype"],
			APIVersion:   parsed["version"],
			specDoc:      wDoc,
			paramCache:   make(map[string]map[string]spec.Parameter),
			paramCacheMu: &sync.RWMutex{},
		}

		return nil
	})

	if len(l.validators) == 0 {
		return ErrSpecDocumentNotFound
	}

	return err
}

func getValidatorKey(resourceType, version string) string {
	return strings.ToLower(strings.Join([]string{resourceType, version}, "-"))
}

func parseSpecFilePath(path string) map[string]string {
	// OpenAPI specs are stored under swagger/ directory structure based on this spec - https://github.com/Azure/azure-rest-api-specs/wiki#directory-structure
	// This regex extracts the information from the filepath.
	re := regexp.MustCompile(".*specification\\/(?P<productname>.+)\\/resource-manager\\/(?P<provider>.+)\\/(?P<state>.+)\\/(?P<version>.+)\\/(?P<resourcetype>.+)\\.json$")
	values := re.FindStringSubmatch(path)
	keys := re.SubexpNames()
	d := map[string]string{}
	for i := 1; i < len(keys); i++ {
		d[keys[i]] = strings.ToLower(values[i])
	}
	return d
}
