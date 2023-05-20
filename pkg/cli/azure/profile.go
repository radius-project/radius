// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
