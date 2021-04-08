// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environment

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/web/mgmt/web"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/radius/pkg/rad/azcli"
)

func (env *TestEnvironment) DeployRP(ctx context.Context, auth autorest.Authorizer) error {
	if os.Getenv("RP_DEPLOY") != "true" {
		fmt.Printf("skipping RP deployment because RP_DEPLOY='%v'\n", os.Getenv("RP_DEPLOY"))
		return nil
	}

	image := os.Getenv("RP_IMAGE")
	if image == "" {
		return fmt.Errorf("Cannot deploy RP image, RP_IMAGE='%v'", image)
	}

	webc := web.NewAppsClient(env.SubscriptionID)
	webc.Authorizer = auth

	list, err := webc.ListByResourceGroupComplete(ctx, env.ResourceGroup, nil)
	if err != nil {
		return fmt.Errorf("cannot read web sites: %w", err)
	}

	if !list.NotDone() {
		return fmt.Errorf("failed to find website in resource group '%v'", env.ResourceGroup)
	}

	website := *list.Value().Name
	fmt.Printf("found website '%v' in resource group '%v'", website, env.ResourceGroup)

	// This command will update the deployed image
	args := []string{
		"webapp", "config", "container", "set",
		"--resource-group", env.ResourceGroup,
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
		"--resource-group", env.ResourceGroup,
		"--name", website,
	}

	err = azcli.RunCLICommand(args...)
	if err != nil {
		return fmt.Errorf("failed to restart rp: %w", err)
	}

	return nil
}
