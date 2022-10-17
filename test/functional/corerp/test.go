// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package corerp

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/stretchr/testify/require"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/test"
)

func NewCoreRPTestOptions(t *testing.T) CoreRPTestOptions {
	ctx, _ := test.GetContext(t)

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	workspace, err := cli.GetWorkspace(config, "")
	require.NoError(t, err, "failed to read default workspace")

	client, err := connections.DefaultFactory.CreateApplicationsManagementClient(ctx, *workspace)
	require.NoError(t, err, "failed to create ApplicationsManagementClient")

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	require.NoError(t, err)
	var awsClient aws.AWSClient = cloudcontrol.NewFromConfig(cfg)

	return CoreRPTestOptions{
		TestOptions:      test.NewTestOptions(t),
		ManagementClient: client,
		AWSClient:        awsClient,
	}
}

type CoreRPTestOptions struct {
	test.TestOptions
	ManagementClient clients.ApplicationsManagementClient
	AWSClient        aws.AWSClient
}
