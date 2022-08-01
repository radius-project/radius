// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const (
	feedbackFile = "feedback.zip"
)

var feedbackCmd = &cobra.Command{
	Use:   "feedback",
	Short: "Captures information about the current Radius Workspace for debugging and diagnostics. Creates a ZIP file of logs in the current directory.",
	Long:  `Captures information about the current Radius Workspace for debugging and diagnostics. Creates a ZIP file of logs in the current directory.`,
	RunE:  feedback,
}

func init() {
	RootCmd.AddCommand(feedbackCmd)
	feedbackCmd.PersistentFlags().StringP("workspace", "w", "", "The workspace name")
}

func feedback(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())

	w, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	connection, err := w.Connect()
	if err != nil {
		return err
	}

	c := connection.(*workspaces.KubernetesConnection)

	// k8sClient, err := // connections.DefaultFactory.CreateK8sClient(cmd.Context(), *workspace)
	k8sClient, _, err := kubernetes.CreateTypedClient(c.Context)
	if err != nil {
		return err
	}

	fmt.Printf("Capturing logs from the Radius workspace \"%s\"\n", w.Name)

	pods, err := k8sClient.CoreV1().Pods("radius-system").List(cmd.Context(), v1.ListOptions{})

	if err != nil {
		return err
	}

	tmpdir, _ := ioutil.TempDir("", "radius-feedback")

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

	defer os.RemoveAll(tmpdir)

	file, err := os.Create(feedbackFile)
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

		t, err := io.Copy(headerWriter, file)
		if err != nil {
			return err
		}
		fmt.Println(t)

		return nil
	}

	err = filepath.Walk(tmpdir, walker)

	fmt.Printf("Wrote zip file %s\n", feedbackFile)

	return err
}

func captureIndividualLogs(ctx context.Context, request *rest.Request, cmd *cobra.Command, filename string) {
	stream, err := request.Stream(cmd.Context())
	if err != nil && err == ctx.Err() {
		return
	} else if err != nil {
		fmt.Printf("Error reading log stream for %s. Error was %+v", filename, err)
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
			fmt.Printf("Error reading log stream for %s. Error was %+v", filename, err)
			return
		}

		if numBytes == 0 {
			continue
		}

		_, err = fh.Write(buf[:numBytes])
		if err != nil {
			fmt.Printf("Error writing to %s. Error was %s", filename, err)
			return
		}
	}
}
