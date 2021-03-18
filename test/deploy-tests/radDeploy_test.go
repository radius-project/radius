// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploytests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
)

var (
	appName = "frontend-backend"
)

// Tests application deployment using radius
func TestDeployApplication(t *testing.T) {
	ctx := context.Background()

	// Find a test cluster
	testClusterName, err := findTestCluster(ctx)
	require.NoError(t, err)

	// Schedule test cluster cleanup
	defer cleanup(ctx, t, testClusterName)

	configFilePath := filepath.Join("./", fmt.Sprintf("%s.yaml", testClusterName))
	// Merge the k8s credentials to the cluster
	err = utils.RunRadMergeCredentialsCommand(configFilePath)
	require.NoError(t, err)

	// Deploy bicep template
	cwd, _ := os.Getwd()
	templateFilePath := filepath.Join(cwd, "../", appName, "/azure-bicep/template.bicep")
	err = utils.RunRadDeployCommand(templateFilePath, configFilePath, time.Minute*5)
	require.NoError(t, err)

	expectedPods := validation.PodSet{
		Namespaces: map[string][]validation.Pod{
			"frontend-backend": []validation.Pod{
				validation.NewPodForComponent("frontend-backend", "frontend"),
				validation.NewPodForComponent("frontend-backend", "backend"),
			},
		},
	}

	k8s, err := utils.GetKubernetesClient()
	require.NoError(t, err, "failed to create kubernetes client")

	validation.ValidatePodsRunning(t, k8s, expectedPods)
}

func cleanup(ctx context.Context, t *testing.T, testClusterName string) {
	// Delete the template deployment
	err := utils.RunRadDeleteApplicationsCommand(testClusterName)
	if err != nil {
		t.Log(err.Error())
	}

	releaseTestCluster(ctx, testClusterName)
}
