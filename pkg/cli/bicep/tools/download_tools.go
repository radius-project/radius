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

package tools

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

const (
	// bicepVersion is the pinned version of the Bicep CLI to download.
	// Keep synchronized with build/tools.yaml; upgrades require registry compatibility testing.
	bicepVersion = "v0.46.1"

	// binaryRepo is the name of the remote bicep binary repository
	binaryRepo = "https://github.com/Azure/bicep/releases/download/" + bicepVersion + "/"
)

// validPlatforms is a map of valid platforms to download for. The key is the combination of GOOS and GOARCH.
var validPlatforms = map[string]string{
	"windows-amd64": "bicep-win-x64",
	"windows-arm64": "bicep-win-arm64",
	"linux-amd64":   "bicep-linux-x64",
	"linux-arm64":   "bicep-linux-arm64",
	"darwin-amd64":  "bicep-osx-x64",
	"darwin-arm64":  "bicep-osx-arm64",
}

// GetLocalFilepath returns the local binary file path. It does not verify that the file
// exists on disk.
//

// GetLocalFilepath checks for an override path in an environment variable, and if it exists, returns it. If not, it
// returns the path to the binary in the user's home directory. It returns an error if it cannot find the user's home
// directory or if the filename is invalid.
func GetLocalFilepath(overrideEnvVarName string, binaryName string) (string, error) {
	override, err := getOverridePath(overrideEnvVarName, binaryName)
	if err != nil {
		return "", err
	} else if override != "" {
		return override, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %v", err)
	}

	filename, err := getFilename(binaryName)
	if err != nil {
		return "", err
	}

	return path.Join(home, ".rad", "bin", filename), nil
}

func getOverridePath(overrideEnvVarName string, binaryName string) (string, error) {
	override := os.Getenv(overrideEnvVarName)
	if override == "" {
		// not overridden
		return "", nil
	}

	file, err := os.Stat(override)
	if err != nil {
		return "", fmt.Errorf("cannot locate %s on overridden path %s: %v", binaryName, override, err)
	}

	if !file.IsDir() {
		return override, nil
	}

	filename, err := getFilename(binaryName)
	if err != nil {
		return "", err
	}
	override = path.Join(override, filename)
	_, err = os.Stat(override)
	if err != nil {
		return "override", fmt.Errorf("cannot locate %s on overridden path %s: %v", binaryName, override, err)
	}

	return override, nil
}

// GetValidPlatform returns the valid platform for the current OS and architecture.
//

// GetValidPlatform checks if the given OS and architecture combination is supported and returns the corresponding
// platform string if it is, or an error if it is not.
func GetValidPlatform(currentOS, currentArch string) (string, error) {
	platform, ok := validPlatforms[currentOS+"-"+currentArch]
	if !ok {
		return "", fmt.Errorf("unsupported platform %s/%s", currentOS, currentArch)
	}
	return platform, nil
}

// DownloadToFolder downloads Bicep, replacing the destination only after a successful download.
func DownloadToFolder(filepath string) error {
	binary, err := GetValidPlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	binaryName, err := getFilename(binary)
	if err != nil {
		return err
	}

	return downloadToFolder(filepath, binaryRepo+binaryName)
}

func downloadToFolder(filename, downloadURL string) error {
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download bicep from %s: %w", downloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download bicep from %s: unexpected HTTP status %s", downloadURL, resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(filename), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create folder %s: %w", filepath.Dir(filename), err)
	}

	// Keep the installed binary intact if the download or file write fails.
	bicepBinary, err := os.CreateTemp(filepath.Dir(filename), ".bicep-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary bicep file: %w", err)
	}
	defer os.Remove(bicepBinary.Name())
	defer bicepBinary.Close()

	if _, err := io.Copy(bicepBinary, resp.Body); err != nil {
		return fmt.Errorf("failed to download bicep from %s: %w", downloadURL, err)
	}
	if err := bicepBinary.Chmod(0o755); err != nil {
		return fmt.Errorf("failed to change permissions for %s: %w", filename, err)
	}
	if err := bicepBinary.Close(); err != nil {
		return fmt.Errorf("failed to close downloaded bicep file: %w", err)
	}
	if err := os.Rename(bicepBinary.Name(), filename); err != nil {
		return fmt.Errorf("failed to replace bicep binary %s: %w", filename, err)
	}

	return nil
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
