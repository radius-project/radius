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

package bicep

import (
	"fmt"
	"os"
	"time"

	"github.com/radius-project/radius/pkg/cli/bicep/tools"
)

const (
	radBicepEnvVar                     = "RAD_BICEP"
	radManifestToBicepExtensionEnvVar  = "RAD_MANIFEST_TO_BICEP_EXTENSION"
	binaryName                         = "rad-bicep"
	manifestToBicepExtensionBinaryName = "manifest-to-bicep-extension"
	retryAttempts                      = 10
	retryDelaySecs                     = 5
)

// DownloadOptions represents the options for downloading bicep and manifest-to-bicep extension
type DownloadOptions struct {
	BicepURL                    string
	ManifestToBicepExtensionURL string
}

func GetBicepFilePath() (string, error) {
	return tools.GetLocalFilepath(radBicepEnvVar, binaryName)
}

func GetManifestToBicepExtensionFilePath() (string, error) {
	return tools.GetLocalFilepath(radManifestToBicepExtensionEnvVar, manifestToBicepExtensionBinaryName)
}

// IsBicepInstalled returns true if our local copy of bicep is installed
//

// IsBicepInstalled checks if the Bicep binary is installed on the local machine and returns a boolean and an error if one occurs.
func IsBicepInstalled() (bool, error) {
	filepath, err := GetBicepFilePath()
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

// DeleteBicep cleans our local copy of bicep
func DeleteBicep() error {
	filepath, err := GetBicepFilePath()
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
//

// DownloadBicep downloads both bicep and manifest-to-bicep extension using default options
func DownloadBicep() error {
	return DownloadBicepTools(DownloadOptions{})
}

// retryDownload executes a download function with retry logic
func retryDownload(toolName string, downloadFunc func() error) error {
	for attempt := 1; attempt <= retryAttempts; attempt++ {
		err := downloadFunc()
		if err != nil {
			if attempt == retryAttempts {
				return fmt.Errorf("failed to download %s after %d attempts: %v", toolName, retryAttempts, err)
			}
			fmt.Printf("Attempt %d failed to download %s: %v\nRetrying in %d seconds...\n", attempt, toolName, err, retryDelaySecs)
			time.Sleep(retryDelaySecs * time.Second)
			continue
		}
		return nil
	}
	return nil
}

// DownloadBicepTools downloads bicep and manifest-to-bicep extension with custom options
func DownloadBicepTools(options DownloadOptions) error {
	// Download bicep CLI
	bicepFilepath, err := GetBicepFilePath()
	if err != nil {
		return err
	}

	err = retryDownload("bicep", func() error {
		return tools.DownloadToFolderWithOptions(bicepFilepath, options.BicepURL)
	})
	if err != nil {
		return err
	}

	// Download manifest-to-bicep-extension CLI
	manifestFilepath, err := tools.GetLocalFilepath(radManifestToBicepExtensionEnvVar, manifestToBicepExtensionBinaryName)
	if err != nil {
		return err
	}

	err = retryDownload("manifest-to-bicep-extension", func() error {
		return tools.DownloadManifestToBicepExtension(manifestFilepath, options.ManifestToBicepExtensionURL)
	})
	if err != nil {
		return err
	}

	return nil
}
