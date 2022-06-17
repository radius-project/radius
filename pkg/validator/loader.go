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
)

const (
	separator = "-"
)

type Loader struct {
	validators   map[string]validator
	providerName string
	specFiles    fs.FS
}

func (l *Loader) Name() string {
	return l.providerName
}

func NewLoader(providerName string, specs fs.FS) *Loader {
	loader := &Loader{
		providerName: providerName,
		validators:   map[string]validator{},
		specFiles:    specs,
	}

	return loader
}

func (l *Loader) GetValidator(resourceType, version string) (Validator, bool) {
	// ARM types are compared case-insensitively
	validator, ok := l.validators[getValidatorKey(resourceType, version)]
	if ok {
		return &validator, true
	}
	return nil, false
}

func (l *Loader) LoadSpec() error {
	err := fs.WalkDir(l.specFiles, ".", func(path string, d fs.DirEntry, _ error) error {
		if d.IsDir() {
			return nil
		}

		if strings.HasPrefix(path, "specification/common-types") {
			return nil
		}

		parsed := parseSpecFilePath(path)
		if namespace, ok := parsed["provider"]; !ok || !strings.EqualFold(namespace, l.providerName) || parsed["resourcetype"] == "global" {
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

		// Expand external references.
		wDoc, err := specDoc.Expanded(&spec.ExpandOptions{
			RelativeBase: path,
			PathLoader: func(path string) (json.RawMessage, error) {
				first := strings.Index(path, "specification")
				data, err := fs.ReadFile(l.specFiles, path[first:])
				return json.RawMessage(data), err
			},
		})

		key := getValidatorKey(parsed["resourcetype"], parsed["version"])

		l.validators[key] = validator{
			TypeName:   parsed["provider"] + "/" + parsed["resourcetype"],
			APIVersion: parsed["version"],
			specDoc:    wDoc,
			params:     make(map[string]map[string]spec.Parameter),
			paramsMu:   &sync.RWMutex{},
		}

		return nil
	})

	if len(l.validators) == 0 {
		return ErrSpecDocumentNotFound
	}

	return err
}

func getValidatorKey(resourceType, version string) string {
	return strings.ToLower(strings.Join([]string{resourceType, version}, separator))
}

func parseSpecFilePath(path string) map[string]string {
	re := regexp.MustCompile(".*specification\\/(?P<name>.+)\\/resource-manager\\/(?P<provider>.+)\\/(?P<state>.+)\\/(?P<version>.+)\\/(?P<resourcetype>.+)\\.json$")
	values := re.FindStringSubmatch(path)
	keys := re.SubexpNames()
	d := map[string]string{}
	for i := 1; i < len(keys); i++ {
		d[keys[i]] = strings.ToLower(values[i])
	}
	return d
}
