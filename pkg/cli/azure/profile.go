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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/dimchansky/utfbom"
)

// Profile represents a Profile from the Azure CLI
type Profile struct {
	InstallationID string         `json:"installationId"`
	Subscriptions  []Subscription `json:"subscriptions"`
}

// Subscription represents a Subscription from the Azure CLI
type Subscription struct {
	EnvironmentName string `json:"environmentName"`
	ID              string `json:"id"`
	IsDefault       bool   `json:"isDefault"`
	Name            string `json:"name"`
	State           string `json:"state"`
	TenantID        string `json:"tenantId"`
	User            *User  `json:"user"`
}

// User represents a User from the Azure CLI
type User struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

const azureProfileJSON = "azureProfile.json"

func configDir() string {
	return os.Getenv("AZURE_CONFIG_DIR")
}

// ProfilePath returns the path where the Azure Profile is stored from the Azure CLI
//

// ProfilePath() checks for the presence of a config directory and returns the path to the azureProfileJSON file in that
// directory, or if the config directory is not present, it returns the path to the azureProfileJSON file in the user's
// home directory. If an error occurs, an error is returned.
func ProfilePath() (string, error) {
	if cfgDir := configDir(); cfgDir != "" {
		return filepath.Join(cfgDir, azureProfileJSON), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return path.Join(homeDir, ".azure", azureProfileJSON), nil
}

// LoadProfile restores a Profile object from a file located at 'path'.
//

// LoadProfile reads a file from the given path, decodes it into a Profile representation and returns the result, or an
// error if the file could not be read or decoded.
func LoadProfile(path string) (result Profile, err error) {
	var contents []byte
	contents, err = os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("failed to open file (%s) while loading token: %v", path, err)
		return
	}
	reader := utfbom.SkipOnly(bytes.NewReader(contents))

	dec := json.NewDecoder(reader)
	if err = dec.Decode(&result); err != nil {
		err = fmt.Errorf("failed to decode contents of file (%s) into a Profile representation: %v", path, err)
		return
	}

	return
}
