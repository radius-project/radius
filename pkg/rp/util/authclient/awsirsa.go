/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authclient

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/google/uuid"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

var _ AuthClient = (*awsIRSA)(nil)

type awsIRSA struct {
	roleARN string
}

func NewAwsIRSA(roleARN string) AuthClient {
	return &awsIRSA{roleARN: roleARN}
}

func (b *awsIRSA) GetAuthClient(ctx context.Context, templatePath string) (remote.Client, error) {
	registryHost, err := getRegistryHostname(templatePath)
	if err != nil {
		return nil, err
	}

	region, err := getECRRegion(registryHost)
	if err != nil {
		return nil, err
	}

	awscfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region))
	if err != nil {
		return nil, errors.New("first error : " + err.Error())
	}

	credsCache := aws.NewCredentialsCache(stscreds.NewWebIdentityRoleProvider(
		sts.NewFromConfig(awscfg),
		b.roleARN,
		stscreds.IdentityTokenFile(ucp_aws.TokenFilePath),
		func(o *stscreds.WebIdentityRoleOptions) {
			o.RoleSessionName = "radius-ecr-" + uuid.New().String()
		},
	))

	ecrCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credsCache),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	ecrClient := ecr.NewFromConfig(ecrCfg)
	authTokenOutput, err := ecrClient.GetAuthorizationToken(ctx, nil)
	if err != nil {
		return nil, err
	}

	if authTokenOutput == nil || len(authTokenOutput.AuthorizationData) == 0 {
		return nil, fmt.Errorf("no authorization data found")
	}

	authData := authTokenOutput.AuthorizationData[0]
	authToken, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode authorization token: %w", err)
	}

	creds := strings.SplitN(string(authToken), ":", 2)
	if len(creds) != 2 {
		return nil, fmt.Errorf("malformed authorization token")
	}

	return &auth.Client{
		Client: retry.DefaultClient,
		Credential: auth.StaticCredential(registryHost, auth.Credential{
			Username: creds[0],
			Password: creds[1],
		}),
	}, nil
}

// getECRRegion extracts the ecr region from the ecr hostname.
// ecr host format: <aws_account_id>.dkr.ecr.<region>.amazonaws.com
func getECRRegion(ecrHost string) (string, error) {
	// Split the hostname to extract the region
	parts := strings.Split(ecrHost, ".")
	if len(parts) < 6 {
		return "", fmt.Errorf("invalid ECR URL format")
	}

	// The region is the third part of the hostname
	return parts[3], nil
}
