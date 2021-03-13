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

	"github.com/Azure/radius/test/utils"
)

var (
	accountName      = "deploytests"
	accountGroupName = "deploytests"
)

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
		err = utils.AcquireStorageContainerLease(ctx, accountName, accountGroupName, testClusterName)
		if err != nil {
			fmt.Printf("Test cluster: %s not available.", testClusterName)
			continue
		}

		// Found test cluster and acquired lease
		return testClusterName, nil
	}

	return "", errors.New("Could not find a test cluster. Retry later")
}

func releaseTestCluster(ctx context.Context, containerName string) {
	utils.BreakStorageContainerLease(ctx, accountName, accountGroupName, containerName)
}
