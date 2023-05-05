// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"fmt"
	"path/filepath"

	"gopkg.in/ini.v1"
)

// # Function Explanation
// 
//	LoadDefaultResourceGroupFromConfig() loads the default resource group from the config file and returns it as a string. 
//	If an error occurs, it returns an error message with details about the cause of the error.
func LoadDefaultResourceGroupFromConfig() (string, error) {
	profilePath, err := ProfilePath()
	if err != nil {
		return "", fmt.Errorf("cannot load azure-cli config: %v", err)
	}
	configPath := filepath.Join(filepath.Dir(profilePath), "config")

	cfg, err := ini.Load(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %v", err)
	}

	return cfg.Section("defaults").Key("group").String(), nil
}
