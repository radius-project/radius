// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package de

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/project-radius/radius/pkg/cli/tools"
)

const radDEEnvVar = "RAD_DE"

// Placeholders are for: channel, platform, filename
const downloadURIFmt = "https://radiuspublic.blob.core.windows.net/tools/de/%s/%s/%s"

// IsBicepInstalled returns true if our local copy of bicep is installed
func IsDEInstalled() (bool, error) {
	filepath, err := tools.GetLocalFilepath()
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

// DeleteBicep cleans our local copy of bicep
func DeleteDE() error {
	filepath, err := tools.GetLocalFilepath()
	if err != nil {
		return err
	}

	err = os.Remove(filepath)
	if err != nil {
		return fmt.Errorf("failed to delete %s: %v", filepath, err)
	}

	return nil
}

// DownloadBicep updates our local copy of bicep
func DownloadDE() error {
	uri, err := tools.GetDownloadURI(downloadURIFmt, "arm-de")
	if err != nil {
		return err
	}

	resp, err := http.Get(uri)
	if err != nil {
		return fmt.Errorf("failed to download bicep: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to download bicep from '%s'with status code: %d", uri, resp.StatusCode)
	}

	filepath, err := GetLocalBicepFilepath()
	if err != nil {
		return err
	}

	// create folders
	err = os.MkdirAll(path.Dir(filepath), os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create folder %s: %v", path.Dir(filepath), err)
	}

	// will truncate the file if it exists
	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", filepath, err)
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %v", filepath, err)
	}

	// get the filemode so we can mark it as executable
	file, err := out.Stat()
	if err != nil {
		return fmt.Errorf("failed to read file attributes %s: %v", filepath, err)
	}

	// make file executable by everyone
	err = out.Chmod(file.Mode() | 0111)
	if err != nil {
		return fmt.Errorf("failed to change permissons for %s: %v", filepath, err)
	}

	return nil
}
