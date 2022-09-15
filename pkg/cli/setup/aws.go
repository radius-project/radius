// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/spf13/cobra"

	radAWS "github.com/project-radius/radius/pkg/cli/aws"
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

func ParseAWSProviderFromArgs(cmd *cobra.Command, interactive bool) (*radAWS.Provider, error) {
	if interactive {
		panic("Not implemented, see https://github.com/project-radius/radius/issues/3655")
	}
	return parseAWSProviderNonInteractive(cmd)

}

func parseAWSProviderNonInteractive(cmd *cobra.Command) (*radAWS.Provider, error) {
	addAWSProvider, err := cmd.Flags().GetBool(AWSProviderFlagName)
	if err != nil {
		return nil, err
	}
	if !addAWSProvider {
		return nil, nil
	}

	keyId, err := cmd.Flags().GetString(AWSProviderAccessKeyIdFlagName)
	if err != nil {
		return nil, err
	}
	secretAccessKey, err := cmd.Flags().GetString(AWSProviderSecretAccessKeyFlagName)
	if err != nil {
		return nil, err
	}

	region, err := cmd.Flags().GetString(AWSProviderRegionFlagName)
	if err != nil {
		return nil, err
	}

	creds := credentials.NewStaticCredentials(keyId, secretAccessKey, "")
	awsConfig := aws.NewConfig().WithCredentials(creds).WithMaxRetries(3)
	mySession := session.Must(session.NewSession(awsConfig))

	client := sts.New(mySession)
	result, err := client.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	awsError := awserr.New("", "", errors.New("")) //placeholder error to be filled by errors.As() below
	if err != nil && errors.As(err, &awsError) {
		errStr := "AWS credential verification failed: %s (AWS ErrorCode: %s)"
		return nil, fmt.Errorf(errStr, awsError.Message(), awsError.Code())
	}

	return &radAWS.Provider{
		AccessKeyId:     keyId,
		SecretAccessKey: secretAccessKey,
		TargetRegion:    region,
		AccountId:       *result.Account,
	}, nil
}
