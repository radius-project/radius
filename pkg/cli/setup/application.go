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
	"fmt"
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

	// PreviewAppBicepTemplate is the app.bicep template used by `rad init --preview`.
	PreviewAppBicepTemplate = `extension radius

@description('The Radius Environment ID. Injected automatically by the rad CLI.')
param environment string

@description('The Radius Application ID. Injected automatically by the rad CLI.')
param application string

resource demo 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'demo'
  properties: {
    environment: environment
    application: application
    containers: {
      demo: {
        image: 'ghcr.io/radius-project/samples/demo:latest'
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
    }
  }
}
` // Trailing newline intentional.

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

	bicepConfigFilepath := filepath.Join(directory, "bicepconfig.json")
	_, err = os.Stat(bicepConfigFilepath)
	if os.IsNotExist(err) {
		err = os.WriteFile(bicepConfigFilepath, []byte(getVersionedBicepConfig()), 0644)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

func getVersionedBicepConfig() string {
	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}

	return fmt.Sprintf(bicepConfigTemplate, tag, tag)
}
