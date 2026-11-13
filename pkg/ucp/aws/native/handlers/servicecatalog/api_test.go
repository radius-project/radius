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

package servicecatalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const apiReference = "https://docs.aws.amazon.com/servicecatalog/latest/APIReference/"

// accept accepts a portfolio share with a fake Service Catalog API backed by store.
func accept(ctx context.Context, store *awsfake.CloudControl, in *servicecatalog.AcceptPortfolioShareInput) error {
	_, err := newFakeAPI(store).AcceptPortfolioShare(ctx, in)
	return err
}

// reject rejects a portfolio share with a fake Service Catalog API backed by store.
func reject(ctx context.Context, store *awsfake.CloudControl, in *servicecatalog.RejectPortfolioShareInput) error {
	_, err := newFakeAPI(store).RejectPortfolioShare(ctx, in)
	return err
}

// listShares lists accepted shares with a fake Service Catalog API backed by store.
func listShares(ctx context.Context, store *awsfake.CloudControl, in *servicecatalog.ListAcceptedPortfolioSharesInput) (*servicecatalog.ListAcceptedPortfolioSharesOutput, error) {
	return newFakeAPI(store).ListAcceptedPortfolioShares(ctx, in)
}

// acceptedPortfolioShareAPI records the Service Catalog calls AcceptedPortfolioShare makes.
var acceptedPortfolioShareAPI = map[string]nativetest.Call{
	"AcceptPortfolioShare": {
		Doc: apiReference + "API_AcceptPortfolioShare.html",
		Rules: []nativetest.Rule{
			{
				Rule: "PortfolioShareType defaults to IMPORTED",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{PortfolioId: aws.String("port-abc123")}); err != nil {
						return err
					}
					out, err := listShares(ctx, store, &servicecatalog.ListAcceptedPortfolioSharesInput{PortfolioShareType: types.PortfolioShareTypeImported})
					if err != nil {
						return err
					}
					if len(out.PortfolioDetails) != 1 {
						return fmt.Errorf("the share isn't listed as imported: %v", out.PortfolioDetails)
					}
					return nil
				},
			},
			{
				Rule: "PortfolioId is 1 to 100 characters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{PortfolioId: aws.String(strings.Repeat("a", 101))})
				},
				Code: "InvalidParametersException",
			},
			{
				Rule: "PortfolioId matches ^[a-zA-Z0-9_\\-]*",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{PortfolioId: aws.String("port abc123")})
				},
				Code: "InvalidParametersException",
			},
			{
				Rule: "AcceptLanguage is at most 100 characters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{
						PortfolioId:    aws.String("port-abc123"),
						AcceptLanguage: aws.String(strings.Repeat("a", 101)),
					})
				},
				Code: "InvalidParametersException",
			},
			{
				Rule: "AcceptLanguage's limit counts characters, not bytes",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					return accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{
						PortfolioId:    aws.String("port-abc123"),
						AcceptLanguage: aws.String(strings.Repeat("界", 100)),
					})
				},
			},
			{
				Rule: "PortfolioShareType is IMPORTED, AWS_SERVICECATALOG, or AWS_ORGANIZATIONS",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{PortfolioId: aws.String("port-abc123"), PortfolioShareType: "SHARED"})
				},
				Code: "InvalidParametersException",
			},
			{
				Rule: "AWS_SERVICECATALOG isn't supported and throws ResourceNotFoundException",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{
						PortfolioId:        aws.String("port-abc123"),
						PortfolioShareType: types.PortfolioShareTypeAwsServicecatalog,
					})
				},
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "the portfolio was shared with the account",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{PortfolioId: aws.String(notShared)})
				},
				Code: "ResourceNotFoundException",
			},
		},
		Unclear: "The page lists jp and zh as language codes but gives no list of valid values, so the fake takes any " +
			"code of up to 100 characters. PortfolioId's pattern has no end anchor; the fake applies it to the whole ID. " +
			"The page doesn't say what AWS does when the share is already accepted; the fake accepts it and changes " +
			"nothing. It lists LimitExceededException without the limit it applies. The fake has no organization " +
			"shares, so it returns ResourceNotFoundException for AWS_ORGANIZATIONS.",
	},
	"ListAcceptedPortfolioShares": {
		Doc: apiReference + "API_ListAcceptedPortfolioShares.html",
		Rules: []nativetest.Rule{
			{
				Rule: "PortfolioShareType defaults to IMPORTED",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{PortfolioId: aws.String("port-abc123")}); err != nil {
						return err
					}
					out, err := listShares(ctx, store, &servicecatalog.ListAcceptedPortfolioSharesInput{})
					if err != nil {
						return err
					}
					if len(out.PortfolioDetails) != 1 {
						return fmt.Errorf("the imported share isn't listed: %v", out.PortfolioDetails)
					}
					return nil
				},
			},
			{
				Rule: "PageSize is 0 to 100",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := listShares(ctx, store, &servicecatalog.ListAcceptedPortfolioSharesInput{PageSize: 101})
					return err
				},
				Code: "InvalidParametersException",
			},
			{
				Rule: "PageToken is at most 2024 characters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := listShares(ctx, store, &servicecatalog.ListAcceptedPortfolioSharesInput{PageToken: aws.String(strings.Repeat("1", 2025))})
					return err
				},
				Code: "InvalidParametersException",
			},
			{
				Rule: "PageToken's pattern allows letters of any language, and its limit counts characters, not bytes",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := listShares(ctx, store, &servicecatalog.ListAcceptedPortfolioSharesInput{PageToken: aws.String(strings.Repeat("界", 2024))})
					return err
				},
			},
			{
				Rule: "AcceptLanguage is at most 100 characters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := listShares(ctx, store, &servicecatalog.ListAcceptedPortfolioSharesInput{AcceptLanguage: aws.String(strings.Repeat("a", 101))})
					return err
				},
				Code: "InvalidParametersException",
			},
			{
				Rule: "PortfolioShareType is IMPORTED, AWS_SERVICECATALOG, or AWS_ORGANIZATIONS",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := listShares(ctx, store, &servicecatalog.ListAcceptedPortfolioSharesInput{PortfolioShareType: "SHARED"})
					return err
				},
				Code: "InvalidParametersException",
			},
		},
		Unclear: "The page calls AWS_SERVICECATALOG a deprecated type without saying what AWS returns for it, and lists " +
			"OperationNotSupportedException without saying when. The fake lists no shares for any type but IMPORTED.",
	},
	"RejectPortfolioShare": {
		Doc: apiReference + "API_RejectPortfolioShare.html",
		Rules: []nativetest.Rule{
			{
				Rule: "PortfolioShareType defaults to IMPORTED",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{PortfolioId: aws.String("port-abc123")}); err != nil {
						return err
					}
					return reject(ctx, store, &servicecatalog.RejectPortfolioShareInput{PortfolioId: aws.String("port-abc123")})
				},
			},
			{
				Rule: "AWS_SERVICECATALOG isn't supported and throws ResourceNotFoundException",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := accept(ctx, store, &servicecatalog.AcceptPortfolioShareInput{PortfolioId: aws.String("port-abc123")}); err != nil {
						return err
					}
					return reject(ctx, store, &servicecatalog.RejectPortfolioShareInput{
						PortfolioId:        aws.String("port-abc123"),
						PortfolioShareType: types.PortfolioShareTypeAwsServicecatalog,
					})
				},
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "the portfolio was shared with the account",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return reject(ctx, store, &servicecatalog.RejectPortfolioShareInput{PortfolioId: aws.String(notShared)})
				},
				Code: "ResourceNotFoundException",
			},
		},
		Unclear: "The page gives the same limits for AcceptLanguage, PortfolioId, and PortfolioShareType as " +
			"AcceptPortfolioShare but names no error for breaking them, so the fake doesn't check them. It doesn't say " +
			"what AWS does for a share that was offered but not accepted; the fake returns ResourceNotFoundException, " +
			"and the handler treats it as already rejected.",
	},
}
