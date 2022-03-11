// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localrp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/mitchellh/go-ps"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/de"
)

var _ clients.DeploymentClient = (*LocalRPDeploymentClient)(nil)

// Local RP Deployment Client to be used for local deployments to Azure environments
type LocalRPDeploymentClient struct {
	InnerClient azure.ARMDeploymentClient
	BindUrl     string
	BackendUrl  string
}

func (dc *LocalRPDeploymentClient) Deploy(ctx context.Context, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
	if dc.BindUrl != "" {
		// channel for completion of the deployment
		completed := make(chan error)

		//
		errs, err := dc.StartDEProcess(ctx, completed)
		if err != nil {
			return clients.DeploymentResult{}, err
		}

		res, err := dc.InnerClient.Deploy(ctx, options)
		completed <- err

		done := <-errs
		if done != nil {
			return res, done
		}

		return res, err
	} else {
		return dc.InnerClient.Deploy(ctx, options)
	}
}

func (dc *LocalRPDeploymentClient) StartDEProcess(ctx context.Context, completed chan error) (chan error, error) {
	// Start the deployment engine and make sure it is up and running.
	installed, err := de.IsDEInstalled()
	if err != nil {
		return nil, err
	}

	if !installed {
		fmt.Println("Deployment Engine is not installed. Installing the latest version...")
		if err = de.DownloadDE(); err != nil {
			return nil, err
		}
	}
	// Cleanup existing processes
	processes, err := ps.Processes()
	if err != nil {
		return nil, err
	}
	for _, p := range processes {
		if p.Executable() == "arm-de" || p.Executable() == "arm-de.exe" {
			fmt.Printf("existing arm-de process with pid: %d, if this is a stray process, consider killing it.\n ", p.Pid())
		}
	}

	executable, err := de.GetDEPath()
	if err != nil {
		return nil, err
	}
	errs := make(chan error)
	go func() {
		// Make sure we close the startup error channel as it is used for pushing the
		defer close(errs)

		args := []string{
			"--",
			fmt.Sprintf("--radiusBackendUri=%s", dc.BackendUrl),
		}
		buf := bytes.Buffer{}

		c := exec.CommandContext(ctx, executable, args...)
		c.Env = append(os.Environ(), fmt.Sprintf("ASPNETCORE_URLS=%s", dc.BindUrl))
		c.Stderr = os.Stderr
		c.Stdout = &buf

		if err != nil {
			errs <- fmt.Errorf("failed to create pipe: %w", err)
			return
		}

		err = c.Start()
		if err != nil {
			errs <- fmt.Errorf("failed executing %q: %w", c.String(), err)
			return
		}

		// Send a nil to the consumer to indicate that the deployment can continue
		errs <- nil

		exitCh := make(chan os.Signal, 1)
		signal.Notify(exitCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		// get completed.
		select {
		case failed := <-completed:
			err = c.Process.Signal(os.Kill)
			if err != nil {
				fmt.Println(fmt.Errorf("failed to send interrupt signal to %q: %w", c.String(), err))
				return
			}

			// read the content
			bytes, err := io.ReadAll(&buf)
			if err != nil {
				fmt.Println(fmt.Errorf("failed to read de output: %w", err))
				return
			}

			if failed != nil {
				fmt.Println(fmt.Errorf("deployment failed: %w output: %s", failed, string(bytes)))
			}
		case <-exitCh:
			err = c.Process.Signal(os.Kill)
			if err != nil {
				fmt.Println(fmt.Errorf("failed to send interrupt signal to %q: %w", c.String(), err))
				return
			}
		}

	}()

	startupErr := <-errs
	if startupErr != nil {
		return errs, startupErr
	}

	return errs, nil
}
