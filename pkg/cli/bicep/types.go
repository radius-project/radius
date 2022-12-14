// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/version"
)

// Interface is the interface for preparing Bicep or ARM-JSON templates for deployment. This interface
// is designed to be called from the CLI and will print output to the console.
type Interface interface {
	PrepareTemplate(filePath string) (map[string]interface{}, error)
}

var _ Interface = (*Impl)(nil)

//go:generate mockgen -destination=./mock_bicep.go -package=bicep -self_package github.com/project-radius/radius/pkg/cli/bicep github.com/project-radius/radius/pkg/cli/bicep Interface

// Impl is the implementation of Interface.
type Impl struct {
}

func (*Impl) PrepareTemplate(filePath string) (map[string]interface{}, error) {
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
		output.LogInfo(fmt.Sprintf("Downloading Bicep for channel %s...", version.Channel()))
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

func ConvertToMapStringInterface(in map[string]map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range in {
		result[k] = v["value"]
	}
	return result
}
