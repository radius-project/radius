// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/aws/aws-sdk-go-v2/aws"
	ucpapi "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	ucpdatamodel "github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/secret"
	ucpsecretp "github.com/project-radius/radius/pkg/ucp/secret/provider"
)

const (
	// DefaultExpireDuration is the default access key expiry duration.
	DefaultExpireDuration = time.Minute * time.Duration(1)
)

// UCPCredentialProvider is used to retrieve credentials via UCP credentials
type UCPCredentialProvider struct {
	options UCPCredentialOptions
}

// UCPCredentialOptions is a configuration for UCPCredentialProvider.
type UCPCredentialOptions struct {
	SecretProvider *ucpsecretp.SecretProvider
	UCPCredClient  *ucpapi.AWSCredentialClient
	Duration       time.Duration
}

// NewUCPCredentialProvider creates UCPCredentialProvider provider to fetch Secret Access key using UCP credential APIs.
func NewUCPCredentialProvider(ucpCredClient *ucpapi.AWSCredentialClient, secretProvider *ucpsecretp.SecretProvider, expireDuration time.Duration) *UCPCredentialProvider {
	o := UCPCredentialOptions{
		SecretProvider: secretProvider,
		UCPCredClient:  ucpCredClient,
		Duration:       expireDuration,
	}

	return &UCPCredentialProvider{options: o}
}

// Retrieve fetches the secret access key using UCP credential API.
func (c *UCPCredentialProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	// 1. Fetch the secret name of aws from UCP
	cred, err := c.options.UCPCredClient.Get(ctx, "aws", "public", "default", &ucpapi.AWSCredentialClientGetOptions{})
	if err != nil {
		return aws.Credentials{}, err
	}

	storage, ok := cred.Properties.GetCredentialResourceProperties().Storage.(*ucpapi.InternalCredentialStorageProperties)
	if !ok {
		return aws.Credentials{}, errors.New("invalid InternalCredentialStorageProperties")
	}

	secretName := to.String(storage.SecretName)
	if secretName == "" {
		return aws.Credentials{}, errors.New("unspecified SecretName for internal storage")
	}

	// 2. Fetch the credential from internal storage (e.g. Kubernetes secret store)
	cli, err := c.options.SecretProvider.GetClient(ctx)
	if err != nil {
		return aws.Credentials{}, err
	}

	s, err := secret.GetSecret[ucpdatamodel.AWSCredentialProperties](ctx, cli, secretName)
	if err != nil {
		return aws.Credentials{}, errors.New("failed to get credential info: " + err.Error())
	}

	if s.AccessKeyID == "" || s.SecretAccessKey == "" {
		return aws.Credentials{}, errors.New("invalid access key info")
	}

	// session name is used to uniquely identify a session. This simply
	// uses unix time in nanoseconds to uniquely identify sessions.
	// TODO: Allow to set session name via options.
	sessionName := strconv.FormatInt(time.Now().UnixNano(), 10)

	value := aws.Credentials{
		AccessKeyID:     s.AccessKeyID,
		SecretAccessKey: s.SecretAccessKey,
		Source:          "Radius UCP",
		SessionToken:    sessionName,
		CanExpire:       true,
		Expires:         time.Now().Add(c.options.Duration),
	}

	return value, nil
}
