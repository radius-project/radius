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

// Package servicecatalog has the native handler for accepting AWS Service Catalog portfolio shares, which Cloud
// Control can't create in some regions.
package servicecatalog

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const acceptedPortfolioShareType = "AWS::ServiceCatalog::AcceptedPortfolioShare"

// API is the part of the Service Catalog client the handler uses.
type API interface {
	AcceptPortfolioShare(ctx context.Context, params *servicecatalog.AcceptPortfolioShareInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.AcceptPortfolioShareOutput, error)
	RejectPortfolioShare(ctx context.Context, params *servicecatalog.RejectPortfolioShareInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.RejectPortfolioShareOutput, error)
	ListAcceptedPortfolioShares(ctx context.Context, params *servicecatalog.ListAcceptedPortfolioSharesInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.ListAcceptedPortfolioSharesOutput, error)
}

var inRegion = awsapi.InRegion[servicecatalog.Options]

var _ native.Handler = (*AcceptedPortfolioShare)(nil)

// AcceptedPortfolioShare serves AWS::ServiceCatalog::AcceptedPortfolioShare: a portfolio another account shared with
// this one, accepted. Its identifier is PortfolioId. Accepting a share AWS already accepted changes nothing, and every
// call takes effect at once. The handler serves shares between accounts (IMPORTED), the shares the type's
// properties can accept.
type AcceptedPortfolioShare struct {
	api API
}

// NewAcceptedPortfolioShare returns the handler for accepted Service Catalog portfolio shares.
func NewAcceptedPortfolioShare(api API) *AcceptedPortfolioShare {
	return &AcceptedPortfolioShare{api: api}
}

// TypeName returns AWS::ServiceCatalog::AcceptedPortfolioShare.
func (h *AcceptedPortfolioShare) TypeName() string {
	return acceptedPortfolioShareType
}

// Options returns how UCP uses the handler.
func (h *AcceptedPortfolioShare) Options() native.Options {
	return native.Options{
		List:     native.ListRegion,
		Recovery: native.RecoverByIdentifier,
	}
}

// accepted returns the IDs of every portfolio the account accepted from another account in the region.
func (h *AcceptedPortfolioShare) accepted(ctx context.Context, region string) ([]string, error) {
	out := []string{}
	var token *string
	for {
		page, err := h.api.ListAcceptedPortfolioShares(ctx, &servicecatalog.ListAcceptedPortfolioSharesInput{
			PageToken:          token,
			PortfolioShareType: types.PortfolioShareTypeImported,
		}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, p := range page.PortfolioDetails {
			out = append(out, aws.ToString(p.Id))
		}
		if token = page.NextPageToken; token == nil {
			return out, nil
		}
	}
}

// Read returns the share's properties. AcceptLanguage is an input only, so it is left out.
func (h *AcceptedPortfolioShare) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	ids, err := h.accepted(ctx, region)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if id == identifier {
			return map[string]any{"PortfolioId": id}, nil
		}
	}
	return nil, native.ErrNotFound
}

// Create accepts the share.
func (h *AcceptedPortfolioShare) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id := awsapi.String(request.Properties, "PortfolioId")
	input := &servicecatalog.AcceptPortfolioShareInput{
		PortfolioId:        aws.String(id),
		PortfolioShareType: types.PortfolioShareTypeImported,
	}
	if language := awsapi.String(request.Properties, "AcceptLanguage"); language != "" {
		input.AcceptLanguage = aws.String(language)
	}
	if _, err := h.api.AcceptPortfolioShare(ctx, input, inRegion(request.Region)); err != nil {
		return "", err
	}
	return id, nil
}

// Update changes nothing, because every property of the type is create-only.
func (h *AcceptedPortfolioShare) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete rejects the share, which removes the portfolio from the account. The other account can share it again.
func (h *AcceptedPortfolioShare) Delete(ctx context.Context, region string, identifier string) error {
	if _, err := h.Read(ctx, region, identifier); err != nil {
		return err
	}
	_, err := h.api.RejectPortfolioShare(ctx, &servicecatalog.RejectPortfolioShareInput{
		PortfolioId:        aws.String(identifier),
		PortfolioShareType: types.PortfolioShareTypeImported,
	}, inRegion(region))
	return awsapi.NotFound(err, "ResourceNotFoundException")
}

// List returns every share the account accepted from another account in the region.
func (h *AcceptedPortfolioShare) List(ctx context.Context, region string) ([]native.Resource, error) {
	ids, err := h.accepted(ctx, region)
	if err != nil {
		return nil, err
	}
	out := []native.Resource{}
	for _, id := range ids {
		out = append(out, native.Resource{Identifier: id, Properties: map[string]any{"PortfolioId": id}})
	}
	return out, nil
}

// Progress reports a create or update as done, because both take effect at once. A delete waits until AWS no longer
// lists the share.
func (h *AcceptedPortfolioShare) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op != native.OperationDelete {
		return native.Progress{Done: true}, nil
	}
	if _, err := h.Read(ctx, region, identifier); err != nil {
		return native.Progress{}, err
	}
	return native.Progress{State: "accepted", Waiting: "deleted"}, nil
}
