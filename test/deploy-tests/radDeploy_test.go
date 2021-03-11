// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploytests

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/radius/test/utils"
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
	if err != nil {
		log.Fatal(err)
	}
	require.NoError(t, err)

	// Deploy bicep template
	cwd, _ := os.Getwd()
	templateFilePath := filepath.Join(cwd, "../", appName, "/azure-bicep/template.bicep")
	err = utils.RunRadDeployCommand(templateFilePath, configFilePath, time.Minute*5)
	if err != nil {
		log.Fatal(err)
	}
	require.NoError(t, err)

	expectedPods := make(map[string]int)
	// Validate pods specified in frontend-backend template are up and running
	expectedPods[appName] = 2
	require.True(t, utils.ValidatePodsRunning(t, expectedPods))
}

func cleanup(ctx context.Context, t *testing.T, testClusterName string) {
	// Delete the template deployment
	utils.RunRadDeleteApplicationsCommand(testClusterName)

	releaseTestCluster(ctx, testClusterName)
}
