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
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/version"
)

// Interface is the interface for preparing Bicep or ARM-JSON templates for deployment. This interface
// is designed to be called from the CLI and will print output to the console.
type Interface interface {
	PrepareTemplate(filePath string) (map[string]any, error)
}

var _ Interface = (*Impl)(nil)

//go:generate mockgen -typed -destination=./mock_bicep.go -package=bicep -self_package github.com/radius-project/radius/pkg/cli/bicep github.com/radius-project/radius/pkg/cli/bicep Interface

// Impl is the implementation of Interface.
type Impl struct {
}

// PrepareTemplate checks if the file is a .json or .bicep file, downloads Bicep if it is not installed, checks if the file
//
//	exists, and builds the template if it does. It returns a map of strings to any and an error if one occurs.
func (*Impl) PrepareTemplate(filePath string) (map[string]any, error) {
	if strings.EqualFold(path.Ext(filePath), ".json") {
		return ReadARMJSON(filePath)
	} else if !strings.EqualFold(path.Ext(filePath), ".bicep") {
		return nil, fmt.Errorf("the provided file %q must be a .json or .bicep file", filePath)
	}

	ok, err := IsBicepInstalled()
	if err != nil {
		return nil, fmt.Errorf("failed to find rad-bicep: %w", err)
	}

	if !ok {
		output.LogInfo("Downloading Bicep for channel %s...", version.Channel())
		err = DownloadBicep()
		if err != nil {
			return nil, fmt.Errorf("failed to download rad-bicep: %w", err)
		}
	}

	// Check the file manually so we can control the error message.
	_, err = os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not find file: %w", err)
	}

	step := output.BeginStep("Building %s...", filePath)
	template, err := Build(filePath)
	if err != nil {
		return nil, err
	}

	output.CompleteStep(step)
	return template, nil
}

// ConvertToMapStringInterface takes in a map of strings to maps of strings to any type and returns a map of strings to any
//
//	type, with the values of the inner maps being the values of the returned map. No errors are returned.
func ConvertToMapStringInterface(in map[string]map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range in {
		result[k] = v["value"]
	}
	return result
}
