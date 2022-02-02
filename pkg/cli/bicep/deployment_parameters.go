// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/project-radius/radius/pkg/cli/clients"
)

// ParameterParser is used to parse the parameters as part of the `rad deploy` command. See the docs for `rad deploy` for examples
// of what we need to support here.
type ParameterParser struct {
	FileSystem fs.FS
}

type OSFileSystem struct {
}

func (OSFileSystem) Open(name string) (fs.File, error) {
	return os.Open(name)
}

func (pp ParameterParser) Parse(inputs ...string) (clients.DeploymentParameters, error) {
	output := clients.DeploymentParameters{}
	for _, input := range inputs {
		// Parameters get merged with the later ones taking precendence. ParseSingleParameter handles
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

		type ParameterFile struct {
			Parameters clients.DeploymentParameters `json:"parameters"`
		}

		data := ParameterFile{}
		err = json.Unmarshal(b, &data)
		if err != nil {
			return err
		}

		pp.mergeParameters(output, data.Parameters)
		return nil
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

		var data interface{}
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

func (pp ParameterParser) mergeParameters(output clients.DeploymentParameters, input clients.DeploymentParameters) {
	// We intentionally overwrite duplicates.
	for k, v := range input {
		output[k] = v
	}
}

func (pp ParameterParser) mergeSingleParameter(output clients.DeploymentParameters, name string, input interface{}) {
	// We intentionally overwrite duplicates.
	output[name] = NewParameter(input)
}

func NewParameter(value interface{}) map[string]interface{} {
	return map[string]interface{}{
		"value": value,
	}
}
