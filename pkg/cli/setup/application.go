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

package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/pkg/version"
)

const (
	// AppBicepTemplate is the app.bicep template used by `rad init`.
	AppBicepTemplate = `extension radius

@description('The Radius Application ID. Injected automatically by the rad CLI.')
param application string

resource demo 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'demo'
  properties: {
    application: application
    container: {
      image: 'ghcr.io/radius-project/samples/demo:latest'
      ports: {
        web: {
          containerPort: 3000
        }
      }
    }
  }
}
` // Trailing newline intentional.

	// BicepConfigFileName is the name of the Bicep configuration file written by `rad init`.
	BicepConfigFileName = "bicepconfig.json"

	bicepConfigTemplate = `{
	"extensions": {
		"radius": "br:biceptypes.azurecr.io/radius:%s",
		"aws": "br:biceptypes.azurecr.io/aws:%s"
	}
}`
)

// ScaffoldApplication creates a working sample application in the provided directory.
func ScaffoldApplication(directory string, template string) error {
	// We NEVER overwrite app.bicep or the bicepconfig.json if it exists. We assume the user might have changed it, and don't
	// want them to lose their content.
	appBicepFilepath := filepath.Join(directory, "app.bicep")
	_, err := os.Stat(appBicepFilepath)
	if os.IsNotExist(err) {
		err = os.WriteFile(appBicepFilepath, []byte(template), 0644)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return WriteBicepConfig(directory)
}

// WriteBicepConfig writes the default bicepconfig.json into directory if it does not already exist.
// It never overwrites an existing file because the user may have customized it.
//
// The file is created with O_EXCL so that a file created concurrently by another process is not truncated.
// Any existing entry at that path, including a directory, is treated as already present.
func WriteBicepConfig(directory string) error {
	bicepConfigFilepath := filepath.Join(directory, BicepConfigFileName)
	f, err := os.OpenFile(bicepConfigFilepath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	} else if err != nil {
		// Some platforms report a different error when the path exists but is not a regular file
		// (for example a directory), so treat any existing entry as present.
		if _, statErr := os.Lstat(bicepConfigFilepath); statErr == nil {
			return nil
		}
		return err
	}

	_, err = f.WriteString(GetVersionedBicepConfig())
	if err != nil {
		_ = f.Close()
		_ = os.Remove(bicepConfigFilepath)
		return err
	}

	err = f.Close()
	if err != nil {
		_ = os.Remove(bicepConfigFilepath)
		return err
	}

	return nil
}

// GetVersionedBicepConfig returns the default bicepconfig.json contents with the Radius and AWS
// Bicep extensions pinned to the current release channel.
func GetVersionedBicepConfig() string {
	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}

	return fmt.Sprintf(bicepConfigTemplate, tag, tag)
}
