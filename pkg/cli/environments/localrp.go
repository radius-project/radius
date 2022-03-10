// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/azure/aks"
	"github.com/project-radius/radius/pkg/azure/armauth"
	azclients "github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	k8s "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LocalRPEnvironment represents a local test setup for Azure Cloud Radius environment.
type LocalRPEnvironment struct {
	Name                      string `mapstructure:"name" validate:"required"`
	Kind                      string `mapstructure:"kind" validate:"required"`
	SubscriptionID            string `mapstructure:"subscriptionid" validate:"required"`
	ResourceGroup             string `mapstructure:"resourcegroup" validate:"required"`
	ControlPlaneResourceGroup string `mapstring:"controlplaneresourcegroup" validate:"required"`
	ClusterName               string `mapstructure:"clustername" validate:"required"`
	DefaultApplication        string `mapstructure:"defaultapplication,omitempty"`

	// URL for the Deployment Engine, TODO run this as part of the start of a deployment
	// if no URL is provided.
	APIDeploymentEngineBaseURL string `mapstructure:"apideploymentenginebaseurl"`

	// URL for the local RP
	URL string `mapstructure:"url,omitempty" validate:"required"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain"`
}

func (e *LocalRPEnvironment) GetName() string {
	return e.Name
}

func (e *LocalRPEnvironment) GetKind() string {
	return e.Kind
}

func (e *LocalRPEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *LocalRPEnvironment) GetContainerRegistry() *Registry {
	return nil
}

func (e *LocalRPEnvironment) GetStatusLink() string {
	// If there's a problem generating the status link, we don't want to fail noisily, just skip the link.
	url, err := azure.GenerateAzureEnvUrl(e.SubscriptionID, e.ResourceGroup)
	if err != nil {
		return ""
	}

	return url
}

func (e *LocalRPEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	var deUrl string
	var err error
	completed := make(chan error)

	if e.APIDeploymentEngineBaseURL == "" {
		deUrl, err = e.StartDEProcess(completed)
		if err != nil {
			return nil, err
		}
	} else {
		deUrl = kubernetes.GetBaseUrlForDeploymentEngine(e.APIDeploymentEngineBaseURL)
	}

	auth, err := armauth.GetArmAuthorizer()
	if err != nil {
		return nil, err
	}

	tags := map[string]*string{}

	tags["azureSubscriptionID"] = &e.SubscriptionID
	tags["azureResourceGroup"] = &e.ResourceGroup

	rgClient := azclients.NewGroupsClient(e.SubscriptionID, auth)
	resp, err := rgClient.Get(ctx, e.ResourceGroup)
	if err != nil {
		return nil, err
	}
	tags["azureLocation"] = resp.Location

	dc := azclients.NewDeploymentsClientWithBaseURI(deUrl, e.SubscriptionID)

	// Poll faster than the default, many deployments are quick
	dc.PollingDelay = 5 * time.Second
	dc.Authorizer = auth

	op := azclients.NewOperationsClientWithBaseUri(deUrl, e.SubscriptionID)
	op.PollingDelay = 5 * time.Second
	op.Authorizer = auth

	client := &azure.ARMDeploymentClient{
		Client:           dc,
		OperationsClient: op,
		SubscriptionID:   e.SubscriptionID,
		ResourceGroup:    e.ResourceGroup,
		Tags:             tags,
		Completed:        completed,
	}

	return client, nil
}

func (e *LocalRPEnvironment) StartDEProcess(completed chan error) (string, error) {
	// Start the deployment engine and make sure it is up and running.
	installed, err := de.IsDEInstalled()
	if err != nil {
		return "", err
	}

	if !installed {
		fmt.Println("Deployment Engine is not installed. Installing the latest version...")
		if err = de.DownloadDE(); err != nil {
			return "", err
		}
	}
	// Cleanup existing processes

	// syscall.Kill(, syscall.SIGTERM)

	executable, err := de.GetDEPath()
	if err != nil {
		return "", err
	}
	startupErrs := make(chan error)
	listenUrl := fmt.Sprintf("https://localhost:%d", 5001)
	deUrl := kubernetes.GetBaseUrlForDeploymentEngine(listenUrl)
	go func() {
		defer close(startupErrs)

		args := fmt.Sprintf("-- --radiusBackendUri=%s --ASPNETCORE_URLS=%s", e.URL, deUrl)
		fullCmd := executable + " " + args
		c := exec.Command(executable, args)
		c.Stderr = os.Stderr
		// c.Stdout = os.Stdout
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

		// Wait() will wait for us to finish draining stderr before returning the exit code
		err = c.Wait()
		if err != nil {
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
		return "", startupErr
	}

	return deUrl, nil
}

func (e *LocalRPEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	config, err := aks.GetAKSMonitoringCredentials(ctx, e.SubscriptionID, e.ControlPlaneResourceGroup, e.ClusterName)
	if err != nil {
		return nil, err
	}

	k8sClient, err := k8s.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	client, err := client.New(config, client.Options{Scheme: kubernetes.Scheme})
	if err != nil {
		return nil, err
	}

	azcred := &radclient.AnonymousCredential{}
	con := arm.NewConnection(e.URL, azcred, nil)

	return &azure.AKSDiagnosticsClient{
		KubernetesDiagnosticsClient: kubernetes.KubernetesDiagnosticsClient{
			K8sClient:  k8sClient,
			Client:     client,
			RestConfig: config,
		},
		ResourceClient: *radclient.NewRadiusResourceClient(con, e.SubscriptionID),
		ResourceGroup:  e.ResourceGroup,
		SubscriptionID: e.SubscriptionID,
	}, nil
}

func (e *LocalRPEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	azcred := &radclient.AnonymousCredential{}
	con := arm.NewConnection(e.URL, azcred, nil)

	return &azure.ARMManagementClient{
		Connection:      con,
		ResourceGroup:   e.ResourceGroup,
		SubscriptionID:  e.SubscriptionID,
		EnvironmentName: e.Name,
	}, nil
}
