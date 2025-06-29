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

package manifestbicep

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"runtime"
	"time"

	"github.com/radius-project/radius/pkg/cli/bicep/tools"
	"github.com/radius-project/radius/pkg/cli/clients"
)

const (
	manifestToBicepExtensionEnvVar = "RAD_MANIFEST_TO_BICEP_EXTENSION"
	binaryName                     = "manifest-to-bicep-extension"
	retryAttempts                  = 10
	retryDelaySecs                 = 5
	
	// bicep-tools repository and version
	bicepToolsRepo    = "https://github.com/willdavsmith/bicep-tools/releases/download/v0.2.0/"
	bicepToolsVersion = "v0.2.0"
)

func GetManifestToBicepExtensionFilePath() (string, error) {
	return tools.GetLocalFilepath(manifestToBicepExtensionEnvVar, binaryName)
}

// IsManifestToBicepExtensionInstalled returns true if our local copy of manifest-to-bicep-extension is installed
func IsManifestToBicepExtensionInstalled() (bool, error) {
	filepath, err := GetManifestToBicepExtensionFilePath()
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

// DeleteManifestToBicepExtension cleans our local copy of manifest-to-bicep-extension
func DeleteManifestToBicepExtension() error {
	filepath, err := GetManifestToBicepExtensionFilePath()
	if err != nil {
		return err
	}

	err = os.Remove(filepath)
	if err != nil {
		return fmt.Errorf("failed to delete %s: %v", filepath, err)
	}

	return nil
}

// DownloadManifestToBicepExtension downloads our local copy of manifest-to-bicep-extension
func DownloadManifestToBicepExtension() error {
	filepath, err := GetManifestToBicepExtensionFilePath()
	if err != nil {
		return err
	}

	for attempt := 1; attempt <= retryAttempts; attempt++ {
		success, err := retryDownload(filepath, attempt, retryAttempts)
		if err != nil {
			return err
		}
		if success {
			break
		}
	}

	return nil
}

func retryDownload(filepath string, attempt, retryAttempts int) (bool, error) {
	err := downloadManifestToBicepExtensionToFolder(filepath)
	if err != nil {
		if attempt == retryAttempts {
			return false, fmt.Errorf("failed to download manifest-to-bicep-extension: %v", err)
		}
		fmt.Printf("Attempt %d failed to download manifest-to-bicep-extension: %v\nRetrying...", attempt, err)
		time.Sleep(retryDelaySecs * time.Second)
		return false, nil
	}

	return true, nil
}

// downloadManifestToBicepExtensionToFolder downloads the manifest-to-bicep-extension binary
func downloadManifestToBicepExtensionToFolder(filepath string) error {
	// Create folders
	err := os.MkdirAll(path.Dir(filepath), os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create folder %s: %v", path.Dir(filepath), err)
	}

	// Create the file
	binary, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer binary.Close()

	// Get platform-specific binary name
	binaryName, err := getManifestToBicepExtensionBinaryName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	// Download the binary
	downloadURL := bicepToolsRepo + binaryName
	resp, err := http.Get(downloadURL)
	if clients.Is404Error(err) {
		return fmt.Errorf("unable to locate manifest-to-bicep-extension binary resource %s: %v", downloadURL, err)
	} else if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Write the body to file
	_, err = io.Copy(binary, resp.Body)
	if err != nil {
		return err
	}

	// Get the filemode so we can mark it as executable
	file, err := binary.Stat()
	if err != nil {
		return fmt.Errorf("failed to read file attributes %s: %v", filepath, err)
	}

	// Make file executable by everyone
	err = binary.Chmod(file.Mode() | 0111)
	if err != nil {
		return fmt.Errorf("failed to change permissions for %s: %v", filepath, err)
	}

	return nil
}

// getManifestToBicepExtensionBinaryName returns the binary name for the current platform
// Maps from Radius platform conventions to bicep-tools release asset names
func getManifestToBicepExtensionBinaryName(goos, goarch string) (string, error) {
	platform := goos + "-" + goarch
	
	switch platform {
	case "darwin-amd64":
		return "manifest-to-bicep-extension-darwin-amd64", nil
	case "darwin-arm64":
		return "manifest-to-bicep-extension-darwin-arm64", nil
	case "linux-amd64":
		return "manifest-to-bicep-extension-linux-amd64", nil
	case "linux-arm64":
		return "manifest-to-bicep-extension-linux-arm64", nil
	case "windows-amd64":
		return "manifest-to-bicep-extension-win-amd64.exe", nil
	default:
		return "", fmt.Errorf("unsupported platform %s/%s", goos, goarch)
	}
}