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
	"context"
	"fmt"
	"os"
	"path"
	"runtime"

	credentials "github.com/oras-project/oras-credentials-go"
	"github.com/radius-project/radius/pkg/version"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	retry_lib "oras.land/oras-go/v2/registry/remote/retry"
)

const (
	// binaryRepo is the name of the remote bicep binary repository
	binaryRepo = "ghcr.io/radius-project/radius/bicep/rad-bicep/"
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

// DownloadToFolder creates a folder and a file, uses the ORAS client to copy from the remote repository to the file,
// and makes the file executable by everyone. An error is returned if any of these steps fail.
func DownloadToFolder(filepath string) error {
	// create folders
	err := os.MkdirAll(path.Dir(filepath), os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create folder %s: %v", path.Dir(filepath), err)
	}

	// Create a file store
	fs, err := file.New(path.Dir(filepath))
	if err != nil {
		return fmt.Errorf("failed to create file store %s: %v", filepath, err)
	}
	defer fs.Close()

	ctx := context.Background()
	platform, err := GetValidPlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	// Define remote repository
	repo, err := remote.NewRepository(binaryRepo + platform)
	if err != nil {
		return err
	}

	// Create credentials to authenticate to repository
	ds, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
		AllowPlaintextPut: true,
	})
	if err != nil {
		return err
	}

	repo.Client = &auth.Client{
		Client:     retry_lib.DefaultClient,
		Cache:      auth.DefaultCache,
		Credential: ds.Get,
	}

	// Copy the artifact from the registry into the file store
	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}
	_, err = oras.Copy(ctx, repo, tag, fs, tag, oras.DefaultCopyOptions)
	if err != nil {
		return err
	}

	// Open the folder so we can mark it as executable
	bicepBinary, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filepath, err)
	}

	// get the filemode so we can mark it as executable
	file, err := bicepBinary.Stat()
	if err != nil {
		return fmt.Errorf("failed to read file attributes %s: %v", filepath, err)
	}

	// make file executable by everyone
	err = bicepBinary.Chmod(file.Mode() | 0111)
	if err != nil {
		return fmt.Errorf("failed to change permissions for %s: %v", filepath, err)
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
