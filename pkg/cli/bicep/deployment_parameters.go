/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bicep

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/radius-project/radius/pkg/cli/clients"
)

// ParameterParser is used to parse the parameters as part of the `rad deploy` command. See the docs for `rad deploy` for examples
// of what we need to support here.
type ParameterParser struct {
	FileSystem fs.FS
}

type OSFileSystem struct {
}

type ParameterFile struct {
	Parameters clients.DeploymentParameters `json:"parameters"`
}

// The Open function opens the file specified by the name parameter and returns a file object and an error if the file
// cannot be opened.
func (OSFileSystem) Open(name string) (fs.File, error) {
	return os.Open(name)
}

// ParseFileContents takes in a map of strings and any type and returns a DeploymentParameters object and
// an error if one occurs during the process.
func (pp ParameterParser) ParseFileContents(input map[string]any) (clients.DeploymentParameters, error) {
	output := clients.DeploymentParameters{}

	b, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	err = pp.unmarshalParameters(b, output)
	if err != nil {
		return nil, err
	}

	return output, err
}

// Parse takes in a variable number of strings and returns a DeploymentParameters object and an error, if any. It
// processes each string input and merges the parameters with the later ones taking precedence, and returns an
// error if any of the strings fail to be processed.
func (pp ParameterParser) Parse(inputs ...string) (clients.DeploymentParameters, error) {
	output := clients.DeploymentParameters{}
	for _, input := range inputs {
		// Parameters get merged with the later ones taking precedence. ParseSingleParameter handles
		// this logic.
		err := pp.parseSingle(input, output)
		if err != nil {
			return nil, fmt.Errorf("failed to process parameter %q: %w", input, err)
		}
	}

	return output, nil
}

func (pp ParameterParser) parseSingle(input string, output clients.DeploymentParameters) error {
	// Parameters come in one of three forms:
	//
	// --parameter @foo.json - declares multiple parameters
	// --parameter foo=@bar.json - declares a single parameter as JSON
	// --parameter foo=bar - declares a single parameter with a string value

	if strings.HasPrefix(input, "@") {
		// input is a file that declares multiple parameters
		filePath := strings.TrimPrefix(input, "@")
		b, err := fs.ReadFile(pp.FileSystem, filePath)
		if err != nil {
			return err
		}

		return pp.unmarshalParameters(b, output)
	}

	// If we get here the parameter needs to have a prefix. We'll split the parameter on the first =. This
	// we we avoid quoting issues.
	parts := strings.SplitN(input, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("cannot parse parameter %q", input)
	}

	parameterName := parts[0]
	parameterValue := parts[1]

	if strings.HasPrefix(parameterValue, "@") {
		// input is a file that declares a single parameter
		filePath := strings.TrimPrefix(parameterValue, "@")
		b, err := fs.ReadFile(pp.FileSystem, filePath)
		if err != nil {
			return err
		}

		var data any
		err = json.Unmarshal(b, &data)
		if err != nil {
			return err
		}

		pp.mergeSingleParameter(output, parameterName, data)
		return nil
	}

	// input is an inline string
	pp.mergeSingleParameter(output, parameterName, parameterValue)
	return nil
}

func (pp ParameterParser) unmarshalParameters(b []byte, output clients.DeploymentParameters) error {

	data := ParameterFile{}
	err := json.Unmarshal(b, &data)
	if err != nil {
		return err
	}

	pp.mergeParameters(output, data.Parameters)
	return nil
}

func (pp ParameterParser) mergeParameters(output clients.DeploymentParameters, input clients.DeploymentParameters) {
	// We intentionally overwrite duplicates.
	for k, v := range input {
		output[k] = v
	}
}

func (pp ParameterParser) mergeSingleParameter(output clients.DeploymentParameters, name string, input any) {
	// We intentionally overwrite duplicates.
	output[name] = NewParameter(input)
}

// NewParameter creates a map containing a single key-value pair, where the key is "value" and the value is the parameter
// passed to the function. If the parameter is invalid, an empty map is returned.
func NewParameter(value any) map[string]any {
	return map[string]any{
		"value": value,
	}
}
