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
	"net/http"
	"os"
	"time"

	"github.com/project-radius/radius/pkg/cli/tools"
)

const (
	radBicepEnvVar = "RAD_BICEP"
	binaryName     = "rad-bicep"
	dirPrefix      = "bicep-extensibility"
	retryAttempts  = 10
	retryDelaySecs = 5
)

// IsBicepInstalled returns true if our local copy of bicep is installed
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
func DownloadBicep() error {
	dirPrefix := "bicep-extensibility"
	// Placeholders are for: channel, platform, filename
	downloadURIFmt := fmt.Sprint("https://get.radapp.dev/tools/", dirPrefix, "/%s/%s/%s")

	uri, err := tools.GetDownloadURI(downloadURIFmt, binaryName)
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
	resp, err := http.Get(uri)
	if err != nil {
		if attempt == retryAttempts {
			return false, fmt.Errorf("failed to download bicep: %v", err)
		}
		fmt.Printf("Attempt %d failed to download bicep: %v\nRetrying...", attempt, err)
		time.Sleep(retryDelaySecs * time.Second)
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if attempt == retryAttempts {
			return false, fmt.Errorf("failed to download bicep from '%s' with status code: %d", uri, resp.StatusCode)
		}
		fmt.Printf("Attempt %d failed to download bicep from '%s' with status code: %d\nRetrying...", attempt, uri, resp.StatusCode)
		time.Sleep(retryDelaySecs * time.Second)
		return false, nil
	}

	err = tools.DownloadToFolder(filepath, resp)
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
