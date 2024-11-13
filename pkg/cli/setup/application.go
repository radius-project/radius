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
	appBicepTemplate = `extension radius

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

	radYamlTemplate = `workspace:
  application: %q
` // Trailing newline intentional.

	bicepConfigTemplate = `{
	"experimentalFeaturesEnabled": {
		"extensibility": true
	},
	"extensions": {
		"radius": "br:biceptypes.azurecr.io/radius:%s",
		"aws": "br:biceptypes.azurecr.io/aws:%s"
	}
}`
)

// ScaffoldApplication creates a working sample application in the provided directory
// along with configuration for the application name.
func ScaffoldApplication(directory string, name string) error {
	// Create .rad in the working directory
	err := os.Mkdir(filepath.Join(directory, ".rad"), 0755)
	if os.IsExist(err) {
		// This is fine
	} else if err != nil {
		return err
	}

	// We NEVER overwrite app.bicep or the bicepconfig.json if it exists. We assume the user might have changed it, and don't
	// want them to lose their content.
	//
	// On the other hand, we ALWAYS overwrite rad.yaml if it exists. We assume that the reason why
	// the user is running `rad init` is to populate it.
	appBicepFilepath := filepath.Join(directory, "app.bicep")
	_, err = os.Stat(appBicepFilepath)
	if os.IsNotExist(err) {
		err = os.WriteFile(appBicepFilepath, []byte(appBicepTemplate), 0644)
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

	radYamlFilepath := filepath.Join(directory, ".rad", "rad.yaml")
	err = os.WriteFile(radYamlFilepath, []byte(fmt.Sprintf(radYamlTemplate, name)), 0644)
	if err != nil {
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
