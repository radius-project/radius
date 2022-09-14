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
	AwsProviderFlagName                = "provider-aws"
	AwsProviderAccessKeyIdFlagName     = "provider-aws-access-key-id"
	AwsProviderSecretAccessKeyFlagName = "provider-aws-secret-access-key"
	AwsProviderRegionFlagName          = "provider-aws-region"
)

func RegisterPersistentAwsProviderArgs(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP(
		AwsProviderFlagName,
		"",
		false,
		"Add AWS provider for cloud resources",
	)
	cmd.PersistentFlags().String(
		AwsProviderAccessKeyIdFlagName,
		"",
		"Specifies an AWS access key associated with an IAM user or role",
	)
	cmd.PersistentFlags().String(
		AwsProviderSecretAccessKeyFlagName,
		"",
		"Specifies the secret key associated with the access key. This is essentially the \"password\" for the access key",
	)
	cmd.PersistentFlags().String(
		AwsProviderRegionFlagName,
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
	addAwsProvider, err := cmd.Flags().GetBool(AwsProviderFlagName)
	if err != nil {
		return nil, err
	}
	if !addAwsProvider {
		return nil, nil
	}

	principalKeyId, err := cmd.Flags().GetString(AwsProviderAccessKeyIdFlagName)
	if err != nil {
		return nil, err
	}
	principalSecret, err := cmd.Flags().GetString(AwsProviderSecretAccessKeyFlagName)
	if err != nil {
		return nil, err
	}

	region, err := cmd.Flags().GetString(AwsProviderRegionFlagName)
	if err != nil {
		return nil, err
	}

	return &aws.Provider{
		AccessKeyId:     principalKeyId,
		SecretAccessKey: principalSecret,
		TargetRegion:    region,
	}, nil
}
