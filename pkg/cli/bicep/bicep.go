// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/Azure/radius/pkg/cli/download"
	"github.com/mitchellh/go-homedir"
)

const radBicepEnvVar = "RAD_BICEP"

// IsBicepInstalled returns true if our local copy of bicep is installed
func IsBicepInstalled() (bool, error) {
	filepath, err := GetLocalBicepFilepath()
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
func DeleteBicep() error {
	filepath, err := GetLocalBicepFilepath()
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
func DownloadBicep() error {
	filepath, err := GetLocalBicepFilepath()
	if err != nil {
		return err
	}

	err = download.Binary(context.Background(), "bicep", filepath)
	if err != nil {
		return err
	}

	return nil
}

// GetLocalBicepFilepath returns the local bicep file path. It does not verify that the file
// exists on disk.
func GetLocalBicepFilepath() (string, error) {
	override, err := getBicepOverridePath()
	if err != nil {
		return "", err
	} else if override != "" {
		return override, nil
	}

	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %v", err)
	}

	filename, err := getBicepFilename()
	if err != nil {
		return "", err
	}

	return path.Join(home, ".rad", "bin", filename), nil
}

func getBicepFilename() (string, error) {
	switch runtime.GOOS {
	case "darwin", "linux":
		return "rad-bicep", nil
	case "windows":
		return "rad-bicep.exe", nil
	default:
		return "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func getBicepOverridePath() (string, error) {
	override := os.Getenv(radBicepEnvVar)
	if override == "" {
		// not overridden
		return "", nil
	}

	// Since is a development-only setting, we're cool with being noisy about it.
	fmt.Println("")

	file, err := os.Stat(override)
	if err != nil {
		return "", fmt.Errorf("cannot locate rad-bicep on overridden path %s: %v", override, err)
	}

	if !file.IsDir() {
		// Since is a development-only setting, we're cool with being noisy about it.
		fmt.Printf("rad bicep overridden to %s", override)
		fmt.Println()
		return override, nil
	}

	filename, err := getBicepFilename()
	if err != nil {
		return "", err
	}
	override = path.Join(override, filename)
	_, err = os.Stat(override)
	if err != nil {
		return "override", fmt.Errorf("cannot locate rad-bicep on overridden path %s: %v", override, err)
	}

	// Since is a development-only setting, we're cool with being noisy about it.
	fmt.Printf("rad bicep overridden to %s", override)
	fmt.Println()
	return override, nil
}
