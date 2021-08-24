// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli_test

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/radcli"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func Test_CLI(t *testing.T) {
	ctx, cancel := utils.GetContext(t)
	defer cancel()

	options := azuretest.NewTestOptions(t)

	// We deploy a simple app and then run a variety of different CLI commands on it. Emphasis here
	// is on the commands that aren't tested as part of our main flow.
	//
	// We use nested tests so we can skip them if we've already failed deployment.
	application := "azure-cli"
	template := "testdata/azure-cli.bicep"

	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateFilePath := filepath.Join(cwd, template)
	t.Logf("deploying %s from file %s", application, template)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	err = cli.Deploy(ctx, templateFilePath)
	require.NoErrorf(t, err, "failed to deploy %s", application)
	t.Logf("finished deploying %s from file %s", application, template)

	// Running for the side effect of making sure the pods are started.
	validation.ValidatePodsRunning(ctx, t, options.K8sClient, validation.K8sObjectSet{
		Namespaces: map[string][]validation.K8sObject{
			application: {
				validation.NewK8sObjectForComponent(application, "a"),
				validation.NewK8sObjectForComponent(application, "b"),
			},
		},
	})

	t.Run("Validate rad application show", func(t *testing.T) {
		output, err := cli.ApplicationShow(ctx, application)
		require.NoError(t, err)
		expected := `APPLICATION  PROVISIONING_STATE  HEALTH_STATE
azure-cli                                  
`
		require.Equal(t, expected, output)
	})

	t.Run("Validate rad component list", func(t *testing.T) {
		output, err := cli.ComponentList(ctx, application)
		require.NoError(t, err)

		// Component ordering can vary so we don't assert exact output.
		require.Contains(t, output, "a          radius.dev/Container@v1alpha1")
		require.Contains(t, output, "b          radius.dev/Container@v1alpha1")
	})

	t.Run("Validate rad component show", func(t *testing.T) {
		output, err := cli.ComponentShow(ctx, application, "a")
		require.NoError(t, err)
		expected := `COMPONENT  KIND                           PROVISIONING_STATE  HEALTH_STATE
a          radius.dev/Container@v1alpha1  NotProvisioned      Unhealthy  
`
		require.Equal(t, expected, output)
	})

	t.Run("Validate rad component logs", func(t *testing.T) {
		output, err := cli.ComponentLogs(ctx, application, "a")
		require.NoError(t, err)

		// We don't want to be too fragile so we're not validating the logs in depth
		require.Contains(t, output, "Server running at http://localhost:3000")
	})

	t.Run("Validate rad component expose", func(t *testing.T) {
		port, err := GetAvailablePort()
		require.NoError(t, err)

		// We open a local port-forward and then make a request to it.
		child, cancel := context.WithCancel(ctx)

		done := make(chan error)
		go func() {
			_, err = cli.ComponentExpose(child, application, "a", port, 3000)
			done <- err
		}()

		for i := 0; i < 10; i++ {
			url := fmt.Sprintf("http://localhost:%d/healthz", port)
			t.Logf("making request to %s", url)
			response, err := http.Get(url)
			if err != nil {
				if i == 10-1 {
					// last retry failed, report failure
					require.NoError(t, err, "failed to get connect to component after 10 retries")
				}
				t.Logf("got error %s", err.Error())
				time.Sleep(1 * time.Second)
				continue
			}
			if response.Body != nil {
				defer response.Body.Close()
			}

			if response.StatusCode > 299 || response.StatusCode < 200 {
				if i == 10-1 {
					// last retry failed, report failure
					require.NoError(t, err, "status code was a bad response after 10 retries %d", response.StatusCode)
				}
				t.Logf("got status %d", response.StatusCode)
				time.Sleep(1 * time.Second)
				continue
			}

			content, err := ioutil.ReadAll(response.Body)
			require.NoError(t, err)

			t.Logf("[response] %s", string(content))
			break
		}

		cancel()
		err = <-done

		// The error should be due to cancellation (we can canceled the command).
		require.Equal(t, context.Canceled, err)
	})
}

func GetAvailablePort() (int, error) {
	address, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", address)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
