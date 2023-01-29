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

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk "github.com/project-radius/radius/pkg/sdk/credentials"
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
	// Provider is an UCP credential provider.
	Provider sdk.CredentialProvider[sdk.AWSCredential]

	// Duration is the duration for the secret keys.
	Duration time.Duration
}

// NewUCPCredentialProvider creates UCPCredentialProvider provider to fetch Secret Access key using UCP credential APIs.
func NewUCPCredentialProvider(provider sdk.CredentialProvider[sdk.AWSCredential], expireDuration time.Duration) *UCPCredentialProvider {
	if expireDuration == 0 {
		expireDuration = DefaultExpireDuration
	}

	o := UCPCredentialOptions{
		Provider: provider,
		Duration: expireDuration,
	}

	return &UCPCredentialProvider{options: o}
}

// Retrieve fetches the secret access key using UCP credential API.
func (c *UCPCredentialProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	s, err := c.options.Provider.Fetch(ctx, sdk.AWSPublic, "default")
	if err != nil {
		return aws.Credentials{}, err
	}

	if s.AccessKeyID == "" || s.SecretAccessKey == "" {
		return aws.Credentials{}, errors.New("invalid access key info")
	}

	// session name is used to uniquely identify a session. This simply
	// uses unix time in nanoseconds to uniquely identify sessions.
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
