// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"fmt"
	"os"
	"path"
	"runtime"

	"github.com/mitchellh/go-homedir"
)

func GetLocalWorkingDirectory() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %v", err)
	}

	return path.Join(home, ".rad", "server"), nil
}

func GetLocalKubeConfigPath() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %v", err)
	}

	return path.Join(home, ".rad", "server", ".kcp", "data", "admin.kubeconfig"), nil
}

func getLocalToolFilepath(tool string, envvar string) (string, error) {
	override, err := getToolOverridePath(tool, envvar)
	if err != nil {
		return "", err
	} else if override != "" {
		return override, nil
	}

	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %v", err)
	}

	filename, err := getToolFilename(tool)
	if err != nil {
		return "", err
	}

	return path.Join(home, ".rad", "bin", filename), nil
}

func getToolFilename(tool string) (string, error) {
	if runtime.GOOS == "darwin" {
		return tool, nil
	} else if runtime.GOOS == "linux" {
		return tool, nil
	} else if runtime.GOOS == "windows" {
		return tool + ".exe", nil
	} else {
		return "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func getToolOverridePath(tool string, envvar string) (string, error) {
	override := os.Getenv(envvar)
	if override == "" {
		// not overridden
		return "", nil
	}

	// Since is a development-only setting, we're cool with being noisy about it.
	fmt.Println("")

	file, err := os.Stat(override)
	if err != nil {
		return "", fmt.Errorf("cannot locate %s on overridden path %s: %v", tool, override, err)
	}

	if !file.IsDir() {
		// Since is a development-only setting, we're cool with being noisy about it.
		fmt.Printf("%s overridden to %s", tool, override)
		fmt.Println()
		return override, nil
	}

	filename, err := getToolFilename(tool)
	if err != nil {
		return "", err
	}
	override = path.Join(override, filename)
	_, err = os.Stat(override)
	if err != nil {
		return "override", fmt.Errorf("cannot locate %s on overridden path %s: %v", tool, override, err)
	}

	// Since is a development-only setting, we're cool with being noisy about it.
	fmt.Printf("rad %s overridden to %s", tool, override)
	fmt.Println()
	return override, nil
}
