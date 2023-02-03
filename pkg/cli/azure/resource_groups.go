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
