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
	"regexp"
	"testing"
	"time"

	"github.com/Azure/radius/pkg/cli/objectformats"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/radcli"
	"github.com/Azure/radius/test/testcontext"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

const (
	retries = 10
)

func Test_CLI(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
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
				validation.NewK8sObjectForResource(application, "a"),
				validation.NewK8sObjectForResource(application, "b"),
			},
		},
	})

	t.Run("Validate rad applicationV3 show", func(t *testing.T) {
		output, err := cli.ApplicationShow(ctx, application)
		require.NoError(t, err)
		expected := `APPLICATION
azure-cli
`
		require.Equal(t, objectformats.TrimSpaceMulti(expected), objectformats.TrimSpaceMulti(output))
	})

	t.Run("Validate rad resource list", func(t *testing.T) {
		output, err := cli.ResourceList(ctx, application)
		require.NoError(t, err)

		// Resource ordering can vary so we don't assert exact output.
		require.Regexp(t, `a\s+ContainerComponent`, output)
		require.Regexp(t, `b\s+ContainerComponent`, output)
	})

	t.Run("Validate rad resource show", func(t *testing.T) {
		output, err := cli.ResourceShow(ctx, application, "ContainerComponent", "a")
		require.NoError(t, err)
		// We are more interested in the content and less about the formatting, which
		// is already covered by unit tests. The spaces change depending on the input
		// and it takes very long to get a feedback from CI.
		expected := regexp.MustCompile(`RESOURCE\s+TYPE\s+PROVISIONING_STATE\s+HEALTH_STATE
a\s+ContainerComponent\s+.*Provisioned\s+.*[h|H]ealthy\s*
`)
		match := expected.MatchString(output)
		require.Equal(t, true, match)
	})

	t.Run("Validate rad resoure logs ContainerComponent", func(t *testing.T) {
		output, err := cli.ResourceLogs(ctx, application, "a")
		require.NoError(t, err)

		// We don't want to be too fragile so we're not validating the logs in depth
		require.Contains(t, output, "Server running at http://localhost:3000")
	})

	t.Run("Validate rad resource expose ContainerComponent", func(t *testing.T) {
		port, err := GetAvailablePort()
		require.NoError(t, err)

		// We open a local port-forward and then make a request to it.
		child, cancel := context.WithCancel(ctx)

		done := make(chan error)
		go func() {
			_, err = cli.ResourceExpose(child, application, "a", port, 3000)
			done <- err
		}()

		for i := 0; i < retries; i++ {
			url := fmt.Sprintf("http://localhost:%d/healthz", port)
			t.Logf("making request to %s", url)
			response, err := http.Get(url)
			if err != nil {
				if i == retries-1 {
					// last retry failed, report failure
					require.NoError(t, err, "failed to get connect to resource after %d retries", retries)
				}
				t.Logf("got error %s", err.Error())
				time.Sleep(1 * time.Second)
				continue
			}
			if response.Body != nil {
				defer response.Body.Close()
			}

			if response.StatusCode > 299 || response.StatusCode < 200 {
				if i == retries-1 {
					// last retry failed, report failure
					require.NoError(t, err, "status code was a bad response after %d retries %d", retries, response.StatusCode)
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
