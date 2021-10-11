// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/mattn/go-isatty"
)

func ValidateBicepFile(filePath string) error {
	_, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("could not find file: %w", err)
	}

	if path.Ext(filePath) != ".bicep" {
		return errors.New("file must be a .bicep file")
	}

	return nil
}

func PerformDeployment(ctx context.Context, client clients.DeploymentClient, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
	// Attach to updates from the deployment client
	update := make(chan clients.DeploymentProgressUpdate)
	done := make(chan struct{})
	if isatty.IsTerminal(os.Stdout.Fd()) {
		listener := cli.InteractiveListener{
			UpdateChannel: update,
			DoneChannel:   done,
		}
		options.UpdateChannel = update
		listener.Start()
	} else {
		listener := cli.TextListener{
			UpdateChannel: update,
			DoneChannel:   done,
		}
		options.UpdateChannel = update
		listener.Start()
	}

	result, err := client.Deploy(ctx, options)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	// Avoid overlapping IO with any last second progress-bar updates
	<-done

	return result, nil
}
