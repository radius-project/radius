// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"fmt"
	"github.com/Azure/go-autorest/autorest/azure/cli"
	"gopkg.in/ini.v1"
	"path/filepath"
)

func GetControlPlaneResourceGroup(resourceGroup string) string {
	return "RE-" + resourceGroup
}

func LoadDefaultResourceGroupFromConfig() (string, error) {
	profilePath, err := cli.ProfilePath()
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
