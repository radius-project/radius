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

package cmd

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/output"
	k8slabels "github.com/radius-project/radius/pkg/kubernetes"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const (
	debugLogsFile = "debug-logs.zip"
)

var debugLogsCmd = &cobra.Command{
	Use:   "debug-logs",
	Short: "Capture logs from Radius control plane for debugging and diagnostics.",
	Long: `Capture logs from Radius control plane for debugging and diagnostics.
	
Creates a ZIP file of logs in the current directory.

WARNING Please inspect all logs before sending feedback to confirm no private information is included.
`,
	RunE: debugLogs,
}

func init() {
	RootCmd.AddCommand(debugLogsCmd)
	debugLogsCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}

func debugLogs(cmd *cobra.Command, args []string) error {
	w, err := cli.RequireWorkspace(cmd, ConfigFromContext(cmd.Context()))
	if err != nil {
		return err
	}

	context, ok := w.KubernetesContext()
	if !ok {
		return clierrors.Message("A Kubernetes connection is required.")
	}

	k8sClient, _, err := kubernetes.NewClientset(context)
	if err != nil {
		return err
	}

	output.LogInfo("Capturing logs from the Radius workspace \"%s\"", w.Name)

	pods, err := k8sClient.CoreV1().Pods("radius-system").List(cmd.Context(), v1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", k8slabels.LabelPartOf, k8slabels.ControlPlanePartOfLabelValue),
	})

	if err != nil {
		return err
	}

	if len(pods.Items) == 0 {
		output.LogInfo("Warning: No pods found. Please check that the Radius control plane is running.")
		output.LogInfo("")
		output.LogInfo("Run `helm ls -A` to check if Radius is installed.")
		output.LogInfo("Run `kubectl get pods -A` to check if Radius is running.")
		output.LogInfo("")
		return nil
	}

	tmpdir, err := os.MkdirTemp("", "radius-debug-logs")
	if err != nil {
		return err
	}

	defer os.RemoveAll(tmpdir)

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			options := &corev1.PodLogOptions{
				Container: container.Name,
			}
			request := k8sClient.CoreV1().Pods("radius-system").GetLogs(pod.Name, options)

			filename := fmt.Sprintf("%s/%s.%s.log", tmpdir, pod.Name, container.Name)

			// Ignore errors from this, always try to capture all logs.
			captureIndividualLogs(cmd.Context(), request, cmd, filename)
		}
	}

	file, err := os.Create(debugLogsFile)
	if err != nil {
		return err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Method = zip.Deflate

		if info.IsDir() {
			return nil
		}

		headerWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(headerWriter, file)
		if err != nil {
			return err
		}

		return nil
	}

	err = filepath.Walk(tmpdir, walker)

	output.LogInfo("Wrote zip file %s. Please inspect each log file and remove any private information before sharing feedback.", debugLogsFile)

	return err
}

func captureIndividualLogs(ctx context.Context, request *rest.Request, cmd *cobra.Command, filename string) {
	stream, err := request.Stream(cmd.Context())
	if err != nil && err == ctx.Err() {
		return
	} else if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error reading log stream for %s. Error was %+v", filename, err)
		return
	}
	defer stream.Close()

	fh, err := os.Create(filename)
	if err != nil {
		return
	}
	defer fh.Close()

	buf := make([]byte, 2000)

	for {
		numBytes, err := stream.Read(buf)

		if err == io.EOF {
			break
		}

		if err != nil && err == ctx.Err() {
			return
		} else if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error reading log stream for %s. Error was %+v", filename, err)
			return
		}

		if numBytes == 0 {
			continue
		}

		_, err = fh.Write(buf[:numBytes])
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error writing to %s. Error was %s", filename, err)
			return
		}
	}
}
