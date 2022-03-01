// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package de

import (
	"fmt"
	"net/http"
	"os"

	"github.com/project-radius/radius/pkg/cli/tools"
)

const radDEEnvVar = "RAD_DE"
const binaryName = "arm-de"

// Placeholders are for: channel, platform, filename
const downloadURIFmt = "https://radiuspublic.blob.core.windows.net/tools/de/%s/%s/%s"

// IsDEInstalled returns true if our local copy of arm-de is installed
func IsDEInstalled() (bool, error) {
	filepath, err := tools.GetLocalFilepath(radDEEnvVar, binaryName)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(filepath)
	if err != nil && os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("error checking for %s: %v", filepath, err)
	}

	return true, nil
}

// DeleteDE cleans our local copy of arm-de
func DeleteDE() error {
	filepath, err := tools.GetLocalFilepath(radDEEnvVar, binaryName)
	if err != nil {
		return err
	}

	err = os.Remove(filepath)
	if err != nil {
		return fmt.Errorf("failed to delete %s: %v", filepath, err)
	}

	return nil
}

// DownloadDE updates our local copy of arm-de
func DownloadDE() error {
	uri, err := tools.GetDownloadURI(downloadURIFmt, binaryName)
	if err != nil {
		return err
	}

	resp, err := http.Get(uri)
	if err != nil {
		return fmt.Errorf("failed to download arm-de: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to download arm-de from '%s'with status code: %d", uri, resp.StatusCode)
	}

	filepath, err := tools.GetLocalFilepath(radDEEnvVar, binaryName)
	if err != nil {
		return err
	}
	return tools.DownloadToFolder(filepath, resp)
}
