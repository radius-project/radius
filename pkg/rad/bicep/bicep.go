// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package bicep

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"runtime"

	"github.com/mitchellh/go-homedir"
)

const radBicepEnvVar = "RAD_BICEP"
const downloadURIFmt = "https://radiuspublic.blob.core.windows.net/tools/bicep/edge/%s/%s"

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

// CleanBicep cleans our local copy of bicep
func CleanBicep() error {
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
	uri, err := getDownloadURI()
	if err != nil {
		return err
	}

	resp, err := http.Get(uri)
	if err != nil {
		return fmt.Errorf("failed to download bicep: %v", err)
	}
	defer resp.Body.Close()

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
	if runtime.GOOS == "darwin" {
		return "rad-bicep", nil
	} else if runtime.GOOS == "linux" {
		return "rad-bicep", nil
	} else if runtime.GOOS == "windows" {
		return "rad-bicep.exe", nil
	} else {
		return "", fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func getDownloadURI() (string, error) {
	filename, err := getBicepFilename()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "darwin" {
		return fmt.Sprintf(downloadURIFmt, "macos-x64", filename), nil
	} else if runtime.GOOS == "linux" {
		return fmt.Sprintf(downloadURIFmt, "linux-x64", filename), nil
	} else if runtime.GOOS == "windows" {
		return fmt.Sprintf(downloadURIFmt, "windows-x64", filename), nil
	} else {
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
