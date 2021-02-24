// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var exposeCmd = &cobra.Command{
	Use:   "expose component on local port",
	Short: "Expose local port",
	Long:  `Expose local port`,
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := createClient()
		if err != nil {
			return err
		}

		client, err := client.New(config, client.Options{Scheme: scheme.Scheme})
		if err != nil {
			return err
		}

		service := v1.Service{}
		err = client.Get(cmd.Context(), types.NamespacedName{Namespace: args[0], Name: args[1]}, &service)
		if err != nil {
			return err
		}

		var remotePort *int32
		for _, p := range service.Spec.Ports {
			remotePort = &p.Port
			break
		}

		if remotePort == nil {
			return errors.New("could not find service port")
		}

		var executableName string
		if runtime.GOOS == "windows" {
			executableName = "kubectl.exe"
		} else {
			executableName = "kubectl"
		}

		c := exec.Command(executableName, "port-forward", "-n", args[0], fmt.Sprintf("svc/%v", args[1]), fmt.Sprintf("%v:%v", args[2], *remotePort))
		c.Stderr = os.Stderr
		c.Stdout = os.Stdout
		err = c.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(exposeCmd)
}

func createClient() (*rest.Config, error) {
	var kubeConfig string
	if home := homeDir(); home != "" {
		kubeConfig = filepath.Join(home, ".kube", "config")
	} else {
		return nil, errors.New("no HOME directory, cannot find kubeconfig")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
