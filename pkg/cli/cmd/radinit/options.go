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
package radinit

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/cli"
	cli_aws "github.com/radius-project/radius/pkg/cli/aws"
	"github.com/radius-project/radius/pkg/cli/azure"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

// initOptions holds all of the options that will be used to initialize Radius.
type initOptions struct {
	Cluster        clusterOptions
	Environment    environmentOptions
	CloudProviders cloudProviderOptions
	Recipes        recipePackOptions
	Application    applicationOptions
}

// clusterOptions holds all of the options that will be used to initialize the Kubernetes cluster.
type clusterOptions struct {
	Install   bool
	Namespace string
	Context   string
	Version   string
}

// environmentOptions holds all of the options that will be used to initialize the environment.
//
// NOTE: cloud provider scope information is not included here, it is part of the cloud provider options.
type environmentOptions struct {
	Create    bool
	Name      string
	Namespace string
}

// cloudProviderOptions holds all of the options that will be used to initialize cloud providers.
type cloudProviderOptions struct {
	Azure *azure.Provider
	AWS   *cli_aws.Provider
}

// recipePackOptions holds all of the options that will be used to initialize recipe packs as part of the environment.
type recipePackOptions struct {
	DevRecipes bool
}

// applicationOptions holds all of the options that will be used to initialize an application in the current directory.
type applicationOptions struct {
	Scaffold bool
	Name     string
}

func (r *Runner) enterInitOptions(ctx context.Context) (*initOptions, *workspaces.Workspace, error) {
	options := initOptions{}

	err := r.enterClusterOptions(ctx, &options)
	if err != nil {
		return nil, nil, err
	}

	ws, err := cli.GetWorkspace(r.ConfigHolder.Config, "")
	if err != nil {
		return nil, nil, err
	}

	// Set up a connection so we can list environments.
	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"context": options.Cluster.Context,
			"kind":    workspaces.KindKubernetes,
		},

		// We can't know the scope yet. Setting it up likes this ensures that any code
		// that needs a resource group will fail. After we know the env name we will
		// update this value.
		Scope: "/planes/radius/local",
	}

	err = r.enterEnvironmentOptions(ctx, workspace, &options)
	if err != nil {
		return nil, nil, err
	}

	err = r.enterCloudProviderOptions(ctx, &options)
	if err != nil {
		return nil, nil, err
	}

	err = r.enterApplicationOptions(ctx, &options)
	if err != nil {
		return nil, nil, err
	}

	options.Recipes.DevRecipes = !r.Full

	if ws == nil {
		workspace.Name = options.Environment.Name
	} else {
		workspace.Name = ws.Name
	}
	workspace.Environment = fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Applications.Core/environments/%s", options.Environment.Name, options.Environment.Name)
	workspace.Scope = fmt.Sprintf("/planes/radius/local/resourceGroups/%s", options.Environment.Name)
	return &options, workspace, nil

}
