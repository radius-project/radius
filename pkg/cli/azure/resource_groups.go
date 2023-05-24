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
