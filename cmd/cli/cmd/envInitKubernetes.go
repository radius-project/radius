// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/kubernetes"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/spf13/cobra"
)

//go:embed radius-k8s.yaml
var k8sManifest []byte

var envInitKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Initializes a kubernetes environment",
	Long:  `Initializes a kubernetes environment`,
	RunE: func(cmd *cobra.Command, args []string) error {
		environmentName, err := cmd.Flags().GetString("environment")
		if err != nil {
			return err
		}

		interactive, err := cmd.Flags().GetBool("interactive")
		if err != nil {
			return err
		}

		namespace, err := cmd.Flags().GetString("namespace")
		if err != nil {
			return err
		}

		if interactive {
			namespace, err = prompt.Text("Enter a namespace name:", prompt.EmptyValidator)
			if err != nil {
				return err
			}
		}

		k8sconfig, err := kubernetes.ReadKubeConfig()
		if err != nil {
			return err
		}

		if k8sconfig.CurrentContext == "" {
			return errors.New("no kubernetes context is set")
		}

		context := k8sconfig.Contexts[k8sconfig.CurrentContext]
		if context == nil {
			return fmt.Errorf("kubernetes context '%s' could not be found", k8sconfig.CurrentContext)
		}

		step := logger.BeginStep("Installing Radius...")
		err = runKubectlApply(cmd.Context(), k8sManifest)
		if err != nil {
			return err
		}
		logger.CompleteStep(step)

		config := ConfigFromContext(cmd.Context())

		env, err := rad.ReadEnvironmentSection(config)
		if err != nil {
			return err
		}

		if environmentName == "" {
			environmentName = k8sconfig.CurrentContext
		}

		env.Items[environmentName] = map[string]interface{}{
			"kind":      environments.KindKubernetes,
			"context":   k8sconfig.CurrentContext,
			"namespace": namespace,
		}

		logger.LogInfo("using environment %v", environmentName)
		env.Default = environmentName
		rad.UpdateEnvironmentSection(config, env)

		err = rad.SaveConfig(config)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
	envInitKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInitKubernetesCmd.Flags().StringP("namespace", "n", "default", "The namespace to use for the environment")
}

// runKubectlApply runs a kubectl CLI command with stdout and stderr buffered for logging when there is an error.
func runKubectlApply(ctx context.Context, content []byte) error {
	var executableName string
	var executableArgs []string
	if runtime.GOOS == "windows" {
		// Use shell on windows since az is a script not an executable
		executableName = fmt.Sprintf("%s\\system32\\cmd.exe", os.Getenv("windir"))
		executableArgs = append(executableArgs, "/c", "kubectl")
	} else {
		executableName = "kubectl"
	}

	// kubectl can accept a file via stdin via passing '-f -'.
	// Ex: cat pod.json | kubectl apply -f - would pass pod.json to kubectl apply.
	executableArgs = append(executableArgs, "apply", "-f", "-")
	c := exec.CommandContext(ctx, executableName, executableArgs...)

	buf := bytes.Buffer{}
	stdin, err := c.StdinPipe()
	if err != nil {
		return err
	}

	stdout, err := c.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := c.StderrPipe()
	if err != nil {
		return err
	}

	err = c.Start()
	if err != nil {
		return err
	}

	go func() {
		// ignore errors from copy failing
		_, _ = io.Copy(&buf, stdout)
	}()

	go func() {
		// ignore errors from copy failing
		_, _ = io.Copy(&buf, stderr)
	}()

	writeme := bytes.NewBuffer(content)
	_, err = io.Copy(stdin, writeme)
	if err != nil {
		return err
	}
	stdin.Close()

	err = c.Wait()
	if err != nil {
		text, _ := ioutil.ReadAll(&buf)
		return fmt.Errorf("failed to install radius: %w\noutput: %s", err, string(text))
	}

	return err
}
