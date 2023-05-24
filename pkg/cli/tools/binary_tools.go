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
	"runtime"

	"github.com/project-radius/radius/pkg/version"
)

// validPlatforms is a map of valid platforms to download for. The key is the combination of GOOS and GOARCH.
var validPlatforms = map[string]string{
	"windows-amd64": "windows-x64",
	"linux-amd64":   "linux-x64",
	"linux-arm":     "linux-arm",
	"linux-arm64":   "linux-arm64",
	"darwin-amd64":  "macos-x64",
	"darwin-arm64":  "macos-arm64",
}

// GetLocalFilepath returns the local binary file path. It does not verify that the file
// exists on disk.
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

	// Since is a development-only setting, we're cool with being noisy about it.
	fmt.Println("")

	file, err := os.Stat(override)
	if err != nil {
		return "", fmt.Errorf("cannot locate %s on overridden path %s: %v", binaryName, override, err)
	}

	if !file.IsDir() {
		// Since is a development-only setting, we're cool with being noisy about it.
		fmt.Printf("%s overridden to %s", binaryName, override)
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
		return "override", fmt.Errorf("cannot locate %s on overridden path %s: %v", binaryName, override, err)
	}

	// Since is a development-only setting, we're cool with being noisy about it.
	fmt.Printf("%s overridden to %s", binaryName, override)
	fmt.Println()
	return override, nil
}

// GetValidPlatform returns the valid platform for the current OS and architecture.
func GetValidPlatform(currentOS, currentArch string) (string, error) {
	platform, ok := validPlatforms[currentOS+"-"+currentArch]
	if !ok {
		return "", fmt.Errorf("unsupported platform %s/%s", currentOS, currentArch)
	}
	return platform, nil
}

func GetDownloadURI(downloadURIFmt string, binaryName string) (string, error) {
	filename, err := getFilename(binaryName)
	if err != nil {
		return "", err
	}

	platform, err := GetValidPlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(downloadURIFmt, version.Channel(), platform, filename), nil
}

func DownloadToFolder(filepath string, resp *http.Response) error {
	// create folders
	err := os.MkdirAll(path.Dir(filepath), os.ModePerm)
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
