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

// Fetch gets the AWS credentials from secret storage. It takes in a context, planeName and name and returns
// an AWSCredential or an error if the fakeCredential is nil.
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
