// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radclientv3

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

var _ azcore.TokenCredential = &AnonymousCredential{}

type AnonymousCredential struct {
}

func (*AnonymousCredential) AuthenticationPolicy(options azcore.AuthenticationPolicyOptions) azcore.Policy {
	return azcore.PolicyFunc(func(req *azcore.Request) (*azcore.Response, error) {
		return req.Next()
	})
}

func (a *AnonymousCredential) GetToken(ctx context.Context, options azcore.TokenRequestOptions) (*azcore.AccessToken, error) {
	return nil, nil
}
