// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tools

import (
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/mitchellh/go-homedir"
	"github.com/project-radius/radius/pkg/version"
)

// GetLocalFilepath returns the local bicep file path. It does not verify that the file
// exists on disk.
func GetLocalFilepath(overrideEnvVarName string, binaryName string) (string, error) {
	override, err := getBicepOverridePath(overrideEnvVarName, binaryName)
	if err != nil {
		return "", err
	} else if override != "" {
		return override, nil
	}

	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %v", err)
	}

	filename, err := getFilename(binaryName)
	if err != nil {
		return "", err
	}

	return path.Join(home, ".rad", "bin", filename), nil
}

func getBicepOverridePath(overrideEnvVarName string, binaryName string) (string, error) {
	override := os.Getenv(overrideEnvVarName)
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

	filename, err := getFilename(binaryName)
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

func GetDownloadURI(downloadURIFmt string, binaryName string) (string, error) {
	filename, err := getFilename(binaryName)
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "darwin" {
		return fmt.Sprintf(downloadURIFmt, version.Channel(), "macos-x64", filename), nil
	} else if runtime.GOOS == "linux" {
		return fmt.Sprintf(downloadURIFmt, version.Channel(), "linux-x64", filename), nil
	} else if runtime.GOOS == "windows" {
		return fmt.Sprintf(downloadURIFmt, version.Channel(), "windows-x64", filename), nil
	} else {
		return "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func getFilename(base string) (string, error) {
	switch runtime.GOOS {
	case "darwin", "linux":
		return base, nil
	case "windows":
		return base + ".exe", nil
	default:
		return "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}
