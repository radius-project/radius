// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package step

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/test"
)

type sender struct {
	RoundTripper http.RoundTripper
}

func (s *sender) Do(request *http.Request) (*http.Response, error) {
	return s.RoundTripper.RoundTrip(request)
}

var _ Executor = (*TempCoreRPExecutor)(nil)

type TempCoreRPExecutor struct {
	Description string
	Template    string
	Parameters  []string
}

// TempCoreRPExecutor is a temporary test executor that bypasses the CLI and
// uses a deployment client manually for testing
func NewTempCoreRPExecutor(template string, parameters ...string) *TempCoreRPExecutor {
	return &TempCoreRPExecutor{
		Description: fmt.Sprintf("deploy %s", template),
		Template:    template,
		Parameters:  parameters,
	}
}

func (d *TempCoreRPExecutor) GetDescription() string {
	return d.Description
}

func (d *TempCoreRPExecutor) Execute(ctx context.Context, t *testing.T, options test.TestOptions) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateFilePath := filepath.Join(cwd, d.Template)
	template, err := bicep.Build(templateFilePath)
	require.NoError(t, err)

	url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine("", "", "", true)
	require.NoError(t, err)

	deploymentsClient := clients.NewResourceDeploymentClientWithBaseURI(url)
	deploymentsClient.Sender = &sender{RoundTripper: roundTripper}

	var templateObject interface{}
	err = json.Unmarshal([]byte(template), &templateObject)
	require.NoError(t, err)

	future, err := deploymentsClient.CreateOrUpdate(ctx, "/planes/deployments/local/resourceGroups/default/providers/Microsoft.Resources/deployments/my-deployment", resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Mode:       resources.DeploymentModeIncremental,
			Template:   templateObject,
			Parameters: map[string]interface{}{},
		},
	})
	require.NoError(t, err, "Deployment failed")

	err = future.WaitForCompletionRef(ctx, deploymentsClient.Client)
	require.NoError(t, err, "Deployment failed")

	deployment, err := future.Result(deploymentsClient.DeploymentsClient)
	require.NoError(t, err, "Deployment failed")

	require.Equal(t, 200, deployment.StatusCode)
}
