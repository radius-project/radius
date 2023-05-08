/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package setup

import (
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/spf13/viper"
)

func ValidateWorkspaceUniqueness(config *viper.Viper, overwrite bool) func(string) (bool, string, error) {
	return func(input string) (bool, string, error) {
		if overwrite {
			return true, "", nil // We're overwriting, so don't bother checking.
		}

		found, err := cli.HasWorkspace(config, input)
		if err != nil {
			return false, "", err
		} else if found {
			return false, fmt.Sprintf("the workspace %q already exists. Specify '--force' to overwrite", input), nil
		}

		return true, "", nil
	}
}
