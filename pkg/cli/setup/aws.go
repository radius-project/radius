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
	AWSProviderFlagName                = "provider-aws"
	AWSProviderAccessKeyIdFlagName     = "provider-aws-access-key-id"
	AWSProviderSecretAccessKeyFlagName = "provider-aws-secret-access-key"
	AWSProviderRegionFlagName          = "provider-aws-region"
)

func RegisterPersistentAWSProviderArgs(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolP(
		AWSProviderFlagName,
		"",
		false,
		"Add AWS provider for cloud resources",
	)
	cmd.PersistentFlags().String(
		AWSProviderAccessKeyIdFlagName,
		"",
		"Specifies an AWS access key associated with an IAM user or role",
	)
	cmd.PersistentFlags().String(
		AWSProviderSecretAccessKeyFlagName,
		"",
		"Specifies the secret key associated with the access key. This is essentially the \"password\" for the access key",
	)
	cmd.PersistentFlags().String(
		AWSProviderRegionFlagName,
		"",
		"Specifies the region to be used for resources deployed by this provider",
	)
}

func ParseAWSProviderFromArgs(cmd *cobra.Command, interactive bool) (*aws.Provider, error) {
	if interactive {
		panic("Not implemented, see https://github.com/project-radius/radius/issues/3655")
	}
	return parseAWSProviderNonInteractive(cmd)

}

func parseAWSProviderNonInteractive(cmd *cobra.Command) (*aws.Provider, error) {
	addAWSProvider, err := cmd.Flags().GetBool(AWSProviderFlagName)
	if err != nil {
		return nil, err
	}
	if !addAWSProvider {
		return nil, nil
	}

	principalKeyId, err := cmd.Flags().GetString(AWSProviderAccessKeyIdFlagName)
	if err != nil {
		return nil, err
	}
	principalSecret, err := cmd.Flags().GetString(AWSProviderSecretAccessKeyFlagName)
	if err != nil {
		return nil, err
	}

	region, err := cmd.Flags().GetString(AWSProviderRegionFlagName)
	if err != nil {
		return nil, err
	}

	return &aws.Provider{
		AccessKeyId:     principalKeyId,
		SecretAccessKey: principalSecret,
		TargetRegion:    region,
	}, nil
}
