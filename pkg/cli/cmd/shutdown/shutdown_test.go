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

package shutdown

import (
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	// A workspace with kind=github — shutdown should accept this.
	githubConfig := radcli.LoadConfig(t, `
workspaces:
  default: github-workspace
  items:
    github-workspace:
      connection:
        kind: github
        context: k3d-radius-github
        stateDir: .radius/state
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
      scope: /planes/radius/local/resourceGroups/default
`)

	// A workspace with kind=kubernetes — shutdown should reject this.
	kubernetesConfig := radcli.LoadConfig(t, `
workspaces:
  default: k8s-workspace
  items:
    k8s-workspace:
      connection:
        kind: kubernetes
        context: kind-kind
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
      scope: /planes/radius/local/resourceGroups/default
`)

	testcases := []radcli.ValidateInput{
		{
			Name:          "github workspace valid",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: githubConfig},
		},
		{
			Name:          "github workspace with cleanup flag",
			Input:         []string{"--cleanup"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: githubConfig},
		},
		{
			Name:          "kubernetes workspace invalid",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: kubernetesConfig},
		},
		{
			Name:          "too many args invalid",
			Input:         []string{"extra-arg"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: githubConfig},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_NewRunner(t *testing.T) {
	factory := &framework.Impl{}
	runner := NewRunner(factory)
	runner.ConfigHolder = &framework.ConfigHolder{}

	// Verify the runner is properly constructed.
	if runner.ConfigHolder == nil {
		t.Fatal("ConfigHolder should not be nil")
	}
}
