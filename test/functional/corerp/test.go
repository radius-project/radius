// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package corerp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/test"
)

func NewCoreRPTestOptions(t *testing.T) CoreRPTestOptions {
	ctx, _ := test.GetContext(t)

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	env, err := cli.GetEnvironment(config, "")
	require.NoError(t, err, "failed to read default environment")

	client, err := environments.CreateApplicationsManagementClientWithScope(ctx, env, "default")
	require.NoError(t, err, "failed to create ApplicationsManagementClientWithScope")

	return CoreRPTestOptions{
		TestOptions:      test.NewTestOptions(t),
		ManagementClient: client,
	}
}

type CoreRPTestOptions struct {
	test.TestOptions
	ManagementClient clients.ApplicationsManagementClient
}
