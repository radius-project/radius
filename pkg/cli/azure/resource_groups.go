// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"github.com/Azure/go-autorest/autorest/azure/cli"
	"gopkg.in/ini.v1"
	"fmt"
)

func GetControlPlaneResourceGroup(resourceGroup string) string {
	return "RE-" + resourceGroup
}

func LoadDefaultResourceGroupFromConfig() (string, error) {
	path, err := cli.ProfilePath()
	if err != nil {
		return "", fmt.Errorf("cannot load azure-cli config: %v", err)
	}

	cfg, err := ini.Load(path)
	if err != nil {
        return "", fmt.Errorf("failed to read config file: %v", err)
       
    }

	return cfg.Section("defaults").Key("group").String(), nil
}
