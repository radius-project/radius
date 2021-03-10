// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploytests

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/radius/test/utils"
	"github.com/stretchr/testify/require"
)

var (
	accountName      = "deploytests"
	accountGroupName = "deploytests"
	appName          = "frontend-backend"
)

// Tests application deployment using radius
func TestDeployApplication(t *testing.T) {
	ctx := context.Background()

	// Find a test cluster
	testClusterName, err := findTestCluster(ctx)
	require.NoError(t, err)

	// Schedule test cluster cleanup
	defer cleanup(ctx, t, testClusterName, appName)

	// Deploy bicep template
	cwd, _ := os.Getwd()
	templateFilePath := filepath.Join(cwd, "../", appName, "/azure-bicep/template.bicep")
	configFilePath := filepath.Join("./", fmt.Sprintf("%s.yaml", testClusterName))
	err = utils.RunRadDeployCommand(templateFilePath, configFilePath, time.Minute*5)
	if err != nil {
		log.Fatal(err)
	}
	require.NoError(t, err)

	// Merge the k8s credentials to the cluster
	err = utils.RunRadMergeCredentialsCommand()
	if err != nil {
		log.Fatal(err)
	}
	require.NoError(t, err)

	expectedPods := make(map[string]int)
	// Validate dapr is installed and running
	expectedPods["dapr-system"] = 5
	// Validate pods specified in frontend-backend template are up and running
	expectedPods[appName] = 2
	require.True(t, utils.ValidatePodsRunning(t, expectedPods))
}

func findTestCluster(ctx context.Context) (string, error) {
	file, err := os.Open("deploy-tests-clusters.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		testClusterName := scanner.Text()
		// Check if cluster is in use
		container, _ := utils.GetContainer(ctx, accountName, accountGroupName, testClusterName)
		fmt.Println(container)
		_, err := container.AcquireLease(ctx, "", 60, azblob.ModifiedAccessConditions{})
		if err != nil {
			fmt.Println("Error acquiring lease: " + err.Error())
			// Move on to the next test cluster
			continue
		}

		// Found test cluster and acquired lease
		return testClusterName, nil
	}

	return "", errors.New("Could not find a test cluster. Retry later")
}

func cleanup(ctx context.Context, t *testing.T, testClusterName, namespace string) {
	// Delete namespace
	utils.DeleteNamespace(t, namespace)

	// Break lease on the test cluster to make it available for other tests
	container, _ := utils.GetContainer(ctx, accountName, accountGroupName, testClusterName)
	fmt.Println(container)
	_, err := container.BreakLease(ctx, 60, azblob.ModifiedAccessConditions{})
	if err != nil {
		fmt.Println("Error breaking lease: " + err.Error())
	}
}
