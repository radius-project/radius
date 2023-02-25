// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdk_cred "github.com/project-radius/radius/pkg/ucp/credentials"
)

type mockProvider struct {
	fakeCredential *sdk_cred.AWSCredential
}

// Fetch gets the AWS credentials from secret storage.
func (p *mockProvider) Fetch(ctx context.Context, planeName, name string) (*sdk_cred.AWSCredential, error) {
	if p.fakeCredential == nil {
		return nil, errors.New("failed to fetch credential")
	}
	return p.fakeCredential, nil
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		fakeCredential: &sdk_cred.AWSCredential{
			AccessKeyID:     "fakeid",
			SecretAccessKey: "fakesecretkey",
		},
	}
}

func TestNewUCPCredentialProvider(t *testing.T) {
	p := NewUCPCredentialProvider(newMockProvider(), 0)
	require.Equal(t, DefaultExpireDuration, p.options.Duration)
}

func TestRetrieve(t *testing.T) {
	t.Run("invalid credential", func(t *testing.T) {
		p := newMockProvider()
		cp := NewUCPCredentialProvider(p, DefaultExpireDuration)
		p.fakeCredential.AccessKeyID = ""

		_, err := cp.Retrieve(context.TODO())
		require.Error(t, err)
	})

	t.Run("valid credential", func(t *testing.T) {
		p := newMockProvider()
		cp := NewUCPCredentialProvider(p, DefaultExpireDuration)

		expectedExpiry := time.Now().UTC().Add(DefaultExpireDuration)
		cred, err := cp.Retrieve(context.TODO())
		require.NoError(t, err)

		require.Equal(t, "fakeid", cred.AccessKeyID)
		require.Equal(t, "fakesecretkey", cred.SecretAccessKey)
		require.Equal(t, "radiusucp", cred.Source)
		require.Equal(t, "fakeid", cred.AccessKeyID)
		require.True(t, cred.CanExpire)
		require.GreaterOrEqual(t, cred.Expires.Unix(), expectedExpiry.Unix())
	})
}
