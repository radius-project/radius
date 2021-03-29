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
	"os"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/web/mgmt/web"
	"github.com/Azure/radius/pkg/rad/azcli"
	"github.com/Azure/radius/test/utils"
)

var (
	accountName      = "deploytests"
	accountGroupName = "deploytests"
)

func findTestCluster(ctx context.Context) (string, error) {
	file, err := os.Open("deploy-tests-clusters.txt")
	if err != nil {
		return "", fmt.Errorf("cannot read test cluster manifest: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		testClusterName := scanner.Text()
		// Check if cluster is in use
		err = utils.AcquireStorageContainerLease(ctx, accountName, accountGroupName, testClusterName)
		if err != nil {
			fmt.Printf("Test cluster: %s not available. err: %v\n", testClusterName, err)
			continue
		}

		// Found test cluster and acquired lease
		return testClusterName, nil
	}

	return "", errors.New("Could not find a test cluster. Retry later")
}

func releaseTestCluster(ctx context.Context, containerName string) {
	_ = utils.BreakStorageContainerLease(ctx, accountName, accountGroupName, containerName)
}

func deployRP(ctx context.Context, webc web.AppsClient, resourceGroup string) error {
	if os.Getenv("RP_DEPLOY") != "true" {
		fmt.Printf("skipping RP deployment because RP_DEPLOY='%v'\n", os.Getenv("RP_DEPLOY"))
		return nil
	}

	image := os.Getenv("RP_IMAGE")
	if image == "" {
		return fmt.Errorf("Cannot deploy RP image, RP_IMAGE='%v'", image)
	}

	list, err := webc.ListByResourceGroupComplete(ctx, resourceGroup, nil)
	if err != nil {
		return fmt.Errorf("cannot read web sites: %w", err)
	}

	if !list.NotDone() {
		return fmt.Errorf("failed to find website in resource group '%v'", resourceGroup)
	}

	err = list.NextWithContext(ctx)
	if err != nil {
		return fmt.Errorf("cannot read web sites: %w", err)
	}

	website := *list.Value().Name
	fmt.Printf("found website '%v' in resource group '%v'", website, resourceGroup)

	// This command will update the deployed image
	args := []string{
		"webapp", "config", "container", "set",
		"--resource-group", resourceGroup,
		"--name", website,
		"--docker-custom-image-name", image,
	}

	err = azcli.RunCLICommand(args...)
	if err != nil {
		return fmt.Errorf("failed to update container to %v: %w", image, err)
	}

	// This command will restart the webapp
	args = []string{
		"webapp", "restart",
		"--resource-group", resourceGroup,
		"--name", website,
	}

	err = azcli.RunCLICommand(args...)
	if err != nil {
		return fmt.Errorf("failed to restart rp: %w", err)
	}

	return nil
}
