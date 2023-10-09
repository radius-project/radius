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

	"github.com/radius-project/radius/pkg/cli/tools"
)

const (
	radBicepEnvVar = "RAD_BICEP"
	binaryName     = "rad-bicep"
	binaryRepo     = "ghcr.io/radius-project/radius/bicep/rad-bicep/%s:%s"
	retryAttempts  = 10
	retryDelaySecs = 5
)

// IsBicepInstalled returns true if our local copy of bicep is installed
//

// IsBicepInstalled checks if the Bicep binary is installed on the local machine and returns a boolean and an error if one occurs.
func IsBicepInstalled() (bool, error) {
	filepath, err := tools.GetLocalFilepath(radBicepEnvVar, binaryName)
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
	filepath, err := tools.GetLocalFilepath(radBicepEnvVar, binaryName)
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

// DownloadBicep() attempts to download a file from a given URI and save it to a local filepath, retrying up to 10 times if
// the download fails. If an error occurs, an error is returned.
func DownloadBicep() error {
	// Placeholders in repo are for: platform, channel
	uri, err := tools.GetDownloadURI(binaryRepo)
	if err != nil {
		return err
	}

	filepath, err := tools.GetLocalFilepath(radBicepEnvVar, binaryName)
	if err != nil {
		return err
	}

	retryAttempts := 10
	for attempt := 1; attempt <= retryAttempts; attempt++ {
		success, err := retry(uri, filepath, attempt, retryAttempts)
		if err != nil {
			return err
		}
		if success {
			break
		}
	}

	return nil
}

func retry(uri, filepath string, attempt, retryAttempts int) (bool, error) {
	err := tools.DownloadToFolder(filepath)
	if err != nil {
		if attempt == retryAttempts {
			return false, fmt.Errorf("failed to download bicep: %v", err)
		}
		fmt.Printf("Attempt %d failed to download bicep: %v\nRetrying...", attempt, err)
		time.Sleep(retryDelaySecs * time.Second)
		return false, nil
	}

	return true, nil
}
