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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/setup"
	"github.com/radius-project/radius/pkg/version"
)

// remoteTemplateTimeout bounds how long we wait when downloading a remote template.
const remoteTemplateTimeout = 60 * time.Second

// Interface is the interface for interacting with Bicep.
type Interface interface {
	PrepareTemplate(filePath string) (map[string]any, error)
	Call(args ...string) ([]byte, error)
}

var _ Interface = (*Impl)(nil)

//go:generate go tool mockgen -typed -destination=./mock_bicep.go -package=bicep -self_package github.com/radius-project/radius/pkg/cli/bicep github.com/radius-project/radius/pkg/cli/bicep Interface

// Impl is the implementation of Interface.
type Impl struct {
	FileSystem filesystem.FileSystem
	Output     output.Interface
}

// PrepareTemplate checks if the file is a .json or .bicep file, downloads Bicep if it is not installed, checks if the file
// exists, and builds the template if it does. The file may be a local path or an http(s) URL; remote templates are
// downloaded to a temporary local file first. It returns a map of strings to any and an error if one occurs.
func (i *Impl) PrepareTemplate(filePath string) (map[string]any, error) {
	// A remote URL is downloaded to a temporary local file so it can be read or compiled like a
	// local template. This mirrors the behavior users expect from tools such as kubectl.
	originalPath := filePath
	remote := isRemoteURL(filePath)
	if remote {
		localPath, cleanup, err := i.downloadTemplate(filePath)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		filePath = localPath
	}

	if strings.EqualFold(path.Ext(filePath), ".json") {
		return ReadARMJSON(filePath)
	} else if !strings.EqualFold(path.Ext(filePath), ".bicep") {
		return nil, fmt.Errorf("the provided file %q must be a .json or .bicep file", originalPath)
	}

	ok, err := IsBicepInstalled()
	if err != nil {
		return nil, fmt.Errorf("failed to find bicep: %w", err)
	}

	if !ok {
		i.Output.LogInfo("Downloading Bicep for channel %s...", version.Channel())
		err = DownloadBicep()
		if err != nil {
			return nil, fmt.Errorf("failed to download bicep: %w", err)
		}
	}

	// Check the file manually so we can control the error message.
	_, err = i.FileSystem.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not find file: %w", err)
	}

	step := i.Output.BeginStep("Building %s...", originalPath)
	bytes, err := i.Call("build", "--stdout", filePath)
	if err != nil {
		i.Output.CompleteStep(step)
		if remote {
			// The bicep compiler prints detailed diagnostics to stderr, so keep the wrapper
			// error focused on identifying the remote source rather than guessing the cause.
			return nil, fmt.Errorf("failed to build remote template %q: %w", originalPath, err)
		}
		return nil, fmt.Errorf("failed to build template: %w", err)
	}

	template := map[string]any{}
	err = json.Unmarshal(bytes, &template)
	if err != nil {
		return nil, err
	}

	i.Output.CompleteStep(step)
	return template, nil
}

// isRemoteURL reports whether filePath is an http or https URL. Local paths, including Windows
// paths such as C:\foo.bicep, are not treated as remote URLs.
func isRemoteURL(filePath string) bool {
	parsed, err := url.Parse(filePath)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// downloadTemplate retrieves a remote template referenced by an http(s) URL and writes it to a
// temporary local file so it can be read or compiled like a local template. It returns the local
// file path and a cleanup function that removes the temporary directory.
func (i *Impl) downloadTemplate(templateURL string) (string, func(), error) {
	parsed, err := url.Parse(templateURL)
	if err != nil {
		return "", nil, fmt.Errorf("invalid template URL %q: %w", templateURL, err)
	}

	ext := path.Ext(parsed.Path)
	if !strings.EqualFold(ext, ".bicep") && !strings.EqualFold(ext, ".json") {
		return "", nil, fmt.Errorf("the provided URL %q must reference a .json or .bicep file", templateURL)
	}

	i.Output.LogInfo("Downloading template from %s...", templateURL)

	client := &http.Client{Timeout: remoteTemplateTimeout}
	resp, err := client.Get(templateURL)
	if err != nil {
		return "", nil, fmt.Errorf("failed to download template from %q: %w", templateURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("failed to download template from %q: unexpected status %s", templateURL, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read template from %q: %w", templateURL, err)
	}

	dir, err := i.FileSystem.MkdirTemp("", "rad-remote-template-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary directory for remote template: %w", err)
	}
	cleanup := func() {
		_ = i.FileSystem.RemoveAll(dir)
	}

	// Preserve the original file name so compiler diagnostics reference a recognizable file.
	localPath := filepath.Join(dir, path.Base(parsed.Path))
	if err := i.FileSystem.WriteFile(localPath, body, 0600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to write remote template to temporary file: %w", err)
	}

	// Bicep discovers bicepconfig.json by walking up from the source file's directory, which for a
	// downloaded template is an isolated temp dir. Provide one so extension declarations such as
	// `extension radius` resolve, preferring the user's own config over a generated default.
	if strings.EqualFold(ext, ".bicep") {
		if err := i.writeBicepConfig(dir); err != nil {
			cleanup()
			return "", nil, err
		}
	}

	return localPath, cleanup, nil
}

// writeBicepConfig places a bicepconfig.json in destDir so extension declarations in a downloaded
// template resolve. It reuses the nearest bicepconfig.json found by searching upward from the
// current working directory, falling back to the default Radius extensions configuration.
func (i *Impl) writeBicepConfig(destDir string) error {
	dest := filepath.Join(destDir, "bicepconfig.json")

	if wd, err := os.Getwd(); err == nil {
		if found := findBicepConfig(i.FileSystem, wd); found != "" {
			data, err := i.FileSystem.ReadFile(found)
			if err != nil {
				return fmt.Errorf("failed to read %q: %w", found, err)
			}
			if err := i.FileSystem.WriteFile(dest, data, 0600); err != nil {
				return fmt.Errorf("failed to write bicepconfig.json: %w", err)
			}
			return nil
		}
	}

	if err := i.FileSystem.WriteFile(dest, []byte(setup.GetVersionedBicepConfig()), 0600); err != nil {
		return fmt.Errorf("failed to write bicepconfig.json: %w", err)
	}
	return nil
}

// findBicepConfig walks up from startDir looking for a bicepconfig.json, mirroring how the Bicep
// compiler discovers configuration. It returns "" if none is found.
func findBicepConfig(fs filesystem.FileSystem, startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, "bicepconfig.json")
		if _, err := fs.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Call runs `bicep` with the given arguments.
func (i *Impl) Call(args ...string) ([]byte, error) {
	return runBicepRaw(args...)
}

// ConvertToMapStringInterface takes in a map of strings to maps of strings to any type and returns a map of strings to any
// type, with the values of the inner maps being the values of the returned map. No errors are returned.
func ConvertToMapStringInterface(in map[string]map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range in {
		result[k] = v["value"]
	}
	return result
}
