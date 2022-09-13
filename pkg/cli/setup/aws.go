// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import (
	"github.com/spf13/cobra"

	"github.com/project-radius/radius/pkg/cli/aws"
)

const (
	AWS_PROVIDER_FLAG_NAME        = "provider-aws"
	AWS_PROVIDER_KEY_ID_FLAG_NAME = "provider-aws-access-key-id"
	AWS_PROVIDER_SECRET_FLAG_NAME = "provider-aws-secret-access-key"
	AWS_PROVIDER_REGION_FLAG_NAME = "provider-aws-region"
)

func RegisterPersistentAwsProviderArgs(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP(
		AWS_PROVIDER_FLAG_NAME,
		"",
		false,
		"Add AWS provider for cloud resources",
	)
	cmd.PersistentFlags().String(
		AWS_PROVIDER_KEY_ID_FLAG_NAME,
		"",
		"Specifies an AWS access key associated with an IAM user or role",
	)
	cmd.PersistentFlags().String(
		AWS_PROVIDER_SECRET_FLAG_NAME,
		"",
		"Specifies the secret key associated with the access key. This is essentially the \"password\" for the access key",
	)
	cmd.PersistentFlags().String(
		AWS_PROVIDER_REGION_FLAG_NAME,
		"",
		"Specifies the region to be used for resources deployed by this provider",
	)
}

func ParseAwsProviderFromArgs(cmd *cobra.Command, interactive bool) (*aws.Provider, error) {
	if interactive {
		panic("Not implemented, see https://github.com/project-radius/radius/issues/3655")
	}
	return parseAwsProviderNonInteractive(cmd)

}

func parseAwsProviderNonInteractive(cmd *cobra.Command) (*aws.Provider, error) {
	addAwsProvider, err := cmd.Flags().GetBool(AWS_PROVIDER_FLAG_NAME)
	if err != nil {
		return nil, err
	}
	if !addAwsProvider {
		return nil, nil
	}

	principalKeyId, err := cmd.Flags().GetString(AWS_PROVIDER_KEY_ID_FLAG_NAME)
	if err != nil {
		return nil, err
	}
	principalSecret, err := cmd.Flags().GetString(AWS_PROVIDER_SECRET_FLAG_NAME)
	if err != nil {
		return nil, err
	}

	region, err := cmd.Flags().GetString(AWS_PROVIDER_REGION_FLAG_NAME)
	if err != nil {
		return nil, err
	}

	return &aws.Provider{
		PrincipalKeyId:     principalKeyId,
		PrincipalAccessKey: principalSecret,
		TargetRegion:       region,
	}, nil
}
