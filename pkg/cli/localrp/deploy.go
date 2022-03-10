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
		errs, err := dc.StartDEProcess(completed)
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

func (dc *LocalRPDeploymentClient) StartDEProcess(completed chan error) (chan error, error) {
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
			fmt.Printf("Killing existing arm-de process with pid: %d\n", p.Pid())
			syscall.Kill(p.Pid(), syscall.SIGKILL)
		}
	}

	executable, err := de.GetDEPath()
	if err != nil {
		return nil, err
	}
	startupErrs := make(chan error)
	go func() {
		defer close(startupErrs)

		args := fmt.Sprintf("-- --radiusBackendUri=%s", dc.BackendUrl)
		fullCmd := executable + " " + args
		c := exec.Command(executable, args)
		c.Env = append(c.Env, fmt.Sprintf("ASPNETCORE_URLS=%s", dc.BindUrl))
		c.Stderr = os.Stderr
		stdout, err := c.StdoutPipe()
		if err != nil {
			startupErrs <- fmt.Errorf("failed to create pipe: %w", err)
			return
		}

		err = c.Start()
		if err != nil {
			startupErrs <- fmt.Errorf("failed executing %q: %w", fullCmd, err)
			return
		}

		startupErrs <- nil

		// asyncronously copy to our buffer, we don't really need to observe
		// errors here since it's copying into memory
		buf := bytes.Buffer{}
		go func() {
			_, _ = io.Copy(&buf, stdout)
		}()

		// get completed.
		failed := <-completed
		err = c.Process.Signal(os.Kill)
		if err != nil {
			fmt.Println(fmt.Errorf("failed to send interrupt signal to %q: %w", fullCmd, err))
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

	}()

	startupErr := <-startupErrs
	if startupErr != nil {
		return startupErrs, startupErr
	}

	return startupErrs, nil
}
