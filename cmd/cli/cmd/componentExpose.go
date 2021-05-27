// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

var exposeCmd = &cobra.Command{
	Use:   "expose component",
	Short: "Exposes a component for network traffic",
	Long: `Exposes a port inside a component for network traffic using a local port.
This command is useful for testing components that accept network traffic but are not exposed to the public internet. Exposing a port for testing allows you to send TCP traffic from your local machine to the component.

Press CTRL+C to exit the command and terminate the tunnel.`,
	Example: `# expose port 80 on the 'orders' component of the 'icecream-store' application
# on local port 5000
rad component expose --application icecream-store orders --port 5000 --remote-port 80`,
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := requireEnvironment(cmd)
		if err != nil {
			return err
		}

		application, err := requireApplication(cmd, env)
		if err != nil {
			return err
		}

		component, err := requireComponent(cmd, args)
		if err != nil {
			return err
		}

		localPort, err := cmd.Flags().GetInt("port")
		if err != nil {
			return err
		}

		remotePort, err := cmd.Flags().GetInt("remote-port")
		if err != nil {
			return err
		}

		if remotePort == -1 {
			remotePort = localPort
		}

		config, err := getMonitoringCredentials(cmd.Context(), *env)
		if err != nil {
			return err
		}

		client, err := k8s.NewForConfig(config)
		if err != nil {
			return err
		}

		replica, err := getRunningReplica(cmd.Context(), client, application, component)
		if err != nil {
			return err
		}

		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt)
		defer signal.Stop(signals)

		failed := make(chan error)
		ready := make(chan struct{})
		stop := make(chan struct{}, 1)
		go func() {
			err := runPortforward(config, client, replica, ready, stop, localPort, remotePort)
			failed <- err
		}()

		for {
			select {
			case <-signals:
				// shutting down... wait for socket to close
				close(stop)
				continue
			case err := <-failed:
				if err != nil {
					return fmt.Errorf("failed to port-forward: %w", err)
				}

				return nil
			}
		}
	},
}

func init() {
	componentCmd.AddCommand(exposeCmd)

	exposeCmd.Flags().IntP("port", "p", -1, "specify the local port")
	err := exposeCmd.MarkFlagRequired("port")
	if err != nil {
		panic(err)
	}

	exposeCmd.Flags().IntP("remote-port", "r", -1, "specify the remote port")
}

func getMonitoringCredentials(ctx context.Context, env environments.AzureCloudEnvironment) (*rest.Config, error) {
	armauth, err := azure.GetResourceManagerEndpointAuthorizer()
	if err != nil {
		return nil, err
	}

	// Currently we go to AKS every time to ask for credentials, we don't
	// cache them locally. This could be done in the future, but skipping it for now
	// since it's non-obvious that we'd store credentials in your ~/.rad directory
	mcc := containerservice.NewManagedClustersClient(env.SubscriptionID)
	mcc.Authorizer = armauth

	results, err := mcc.ListClusterMonitoringUserCredentials(ctx, env.ResourceGroup, env.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to list AKS cluster credentials: %w", err)
	}

	if results.Kubeconfigs == nil || len(*results.Kubeconfigs) == 0 {
		return nil, errors.New("failed to list AKS cluster credentials: response did not contain credentials")
	}

	kc := (*results.Kubeconfigs)[0]
	c, err := clientcmd.NewClientConfigFromBytes(*kc.Value)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig was invalid: %w", err)
	}

	restconfig, err := c.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig did not contain client credentials: %w", err)
	}

	return restconfig, nil
}

func getRunningReplica(ctx context.Context, client *k8s.Clientset, application string, component string) (*corev1.Pod, error) {
	// Right now this connects to a pod related to a component. We can find the pods with the labels
	// and then choose one that's in the running state.
	pods, err := client.CoreV1().Pods(application).List(ctx, v1.ListOptions{
		LabelSelector: labels.FormatLabels(map[string]string{workloads.LabelRadiusComponent: component}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list running replicas for component %v: %w", component, err)
	}

	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("failed to find a running replica for component %v", component)
}

func runPortforward(restconfig *rest.Config, client *k8s.Clientset, replica *corev1.Pod, ready chan struct{}, stop <-chan struct{}, localPort int, remotePort int) error {
	// Build URL so we can open a port-forward via SPDY
	url := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(replica.Namespace).
		Name(replica.Name).
		SubResource("portforward").URL()

	transport, upgrader, err := spdy.RoundTripperFor(restconfig)
	if err != nil {
		return err
	}

	out := ioutil.Discard
	errOut := ioutil.Discard
	if true {
		out = os.Stdout
		errOut = os.Stderr
	}

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)

	fw, err := portforward.NewOnAddresses(dialer, []string{"localhost"}, ports, stop, ready, out, errOut)
	if err != nil {
		return err
	}

	return fw.ForwardPorts()
}
