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

package redshift

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const dataShareAuthorizationType = "AWS::Redshift::DataShareAuthorization"

var (
	_ native.Handler           = (*DataShareAuthorization)(nil)
	_ native.IdentifierEncoder = (*DataShareAuthorization)(nil)

	// dataShareConsumerIdentifier is the form of consumer the handler takes: a 12-digit AWS account ID. AWS also takes
	// keywords such as ADX and DataCatalog/<account ID>, but no run showed that AWS reports them back as sent, which
	// Read relies on to find the authorization. DataCatalog/<account ID> is also the consumer of the INTERNAL data
	// share a namespace registration makes.
	dataShareConsumerIdentifier = regexp.MustCompile(`^[0-9]{12}$`)

	// dataShareAuthorizedStatuses are the statuses of a consumer's association that mean the producer has authorized the data
	// share for it: AUTHORIZED until the consumer associates it, and ACTIVE once it has. Every other status means the
	// authorization is gone: PENDING_AUTHORIZATION is a grant in SQL the producer hasn't authorized yet, DEAUTHORIZED
	// one it removed, and REJECTED one the consumer declined, which the producer can authorize again. AVAILABLE is
	// what the consumer account reports for an authorized data share, never the producer.
	dataShareAuthorizedStatuses = []types.DataShareStatus{types.DataShareStatusAuthorized, types.DataShareStatusActive}
)

// DataShareAuthorization serves AWS::Redshift::DataShareAuthorization: the producer's authorization of a data share
// for one consumer. Its identifier is DataShareArn and ConsumerIdentifier joined with "|", which the template gives.
// It never waits for the consumer: the consumer account accepts the data share separately, such as with
// AWS::Redshift::DataShareConsumerAssociation, and a create is done once the data share is AUTHORIZED.
type DataShareAuthorization struct {
	api DataShareAPI
}

// NewDataShareAuthorization returns the handler for Redshift data share authorizations.
func NewDataShareAuthorization(api DataShareAPI) *DataShareAuthorization {
	return &DataShareAuthorization{api: api}
}

// TypeName returns AWS::Redshift::DataShareAuthorization.
func (h *DataShareAuthorization) TypeName() string {
	return dataShareAuthorizationType
}

// Options returns how UCP uses the handler. Authorizing again is safe: AWS answers with the same authorization.
func (h *DataShareAuthorization) Options() native.Options {
	return native.Options{List: native.ListRegion, Recovery: native.RecoverByIdentifier}
}

// Identifier checks the template's values, so UCP rejects one AWS wouldn't take before any AWS call, and returns
// DataShareArn and ConsumerIdentifier joined with "|", or "" when the template leaves either out.
func (h *DataShareAuthorization) Identifier(properties map[string]any) (string, error) {
	if err := dataShareCheckArn(dataShareAuthorizationType, properties); err != nil {
		return "", err
	}
	if value, ok := properties["ConsumerIdentifier"]; ok && value != nil {
		if s, ok := value.(string); !ok || !dataShareConsumerIdentifier.MatchString(s) {
			return "", dataShareInvalid(dataShareAuthorizationType, "ConsumerIdentifier", "a 12-digit AWS account ID")
		}
	}
	if err := dataShareCheckBools(dataShareAuthorizationType, properties, "AllowWrites"); err != nil {
		return "", err
	}
	arn, consumer := awsapi.String(properties, "DataShareArn"), awsapi.String(properties, "ConsumerIdentifier")
	if arn == "" || consumer == "" {
		return "", nil
	}
	return arn + "|" + consumer, nil
}

// split returns the data share ARN and the consumer of an identifier.
func (h *DataShareAuthorization) split(identifier string) (string, string, error) {
	arn, consumer, _ := strings.Cut(identifier, "|")
	if checked, err := h.Identifier(map[string]any{"DataShareArn": arn, "ConsumerIdentifier": consumer}); err != nil || checked == "" {
		return "", "", &native.Failure{
			Message:     fmt.Sprintf("the identifier %q of an %s must be DataShareArn|ConsumerIdentifier", identifier, dataShareAuthorizationType),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return arn, consumer, nil
}

// find returns the data share with the identifier's ARN and the consumer's association with it, which is nil when the
// data share has none, or when the association is the consumer account's own rather than this account's
// authorization. It returns ErrNotFound when AWS has no such data share, which Redshift also answers for a data share
// it granted to no consumer.
func (h *DataShareAuthorization) find(ctx context.Context, region string, identifier string) (*types.DataShare, *types.DataShareAssociation, error) {
	arn, consumer, err := h.split(identifier)
	if err != nil {
		return nil, nil, err
	}
	out, err := h.api.DescribeDataShares(ctx, &redshift.DescribeDataSharesInput{DataShareArn: aws.String(arn)}, dataShareInRegion(region))
	if err != nil {
		// The ARN is the call's only input, so InvalidDataShareFault means AWS has no such data share.
		return nil, nil, awsapi.NotFound(err, dataShareFault)
	}
	i := slices.IndexFunc(out.DataShares, func(s types.DataShare) bool { return aws.ToString(s.DataShareArn) == arn })
	if i < 0 {
		return nil, nil, native.ErrNotFound
	}
	share, err := dataShareOwn(&out.DataShares[i], identifier)
	if err != nil {
		return nil, nil, err
	}
	a := dataShareAuthorizationOf(share, consumer)
	if a != nil && a.Status == types.DataShareStatusActive {
		produced, err := h.produces(ctx, region, share)
		if err != nil || !produced {
			return share, nil, err
		}
	}
	return share, a, nil
}

// produces reports whether the account produces share. The consumer account reports a data share it associated with
// the entire account as ACTIVE too, with the same consumer identifier, so an ACTIVE association alone doesn't show that
// this account authorized it. AWS answers a producer namespace of another account with InvalidParameterValue.
func (h *DataShareAuthorization) produces(ctx context.Context, region string, share *types.DataShare) (bool, error) {
	pages := redshift.NewDescribeDataSharesForProducerPaginator(h.api, &redshift.DescribeDataSharesForProducerInput{ProducerArn: share.ProducerArn})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, dataShareInRegion(region))
		if awsapi.HasCode(err, dataShareParameterFault, dataShareNamespaceFault) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		if slices.ContainsFunc(page.DataShares, func(s types.DataShare) bool { return aws.ToString(s.DataShareArn) == aws.ToString(share.DataShareArn) }) {
			return true, nil
		}
	}
	return false, nil
}

// dataShareAuthorizationOf returns the producer's association of share with consumer, or nil. The producer's association has
// no ConsumerRegion: AWS reports the consumer account's Region and namespace associations only to the consumer.
func dataShareAuthorizationOf(share *types.DataShare, consumer string) *types.DataShareAssociation {
	for i := range share.DataShareAssociations {
		a := &share.DataShareAssociations[i]
		if aws.ToString(a.ConsumerIdentifier) == consumer && aws.ToString(a.ConsumerRegion) == "" {
			return a
		}
	}
	return nil
}

// dataShareAuthorizationProperties returns the properties of the authorization of share for the consumer a.
func dataShareAuthorizationProperties(share *types.DataShare, a *types.DataShareAssociation) map[string]any {
	properties := map[string]any{
		"DataShareArn":       aws.ToString(share.DataShareArn),
		"ConsumerIdentifier": aws.ToString(a.ConsumerIdentifier),
		"Status":             string(a.Status),
		// The API reference marks ProducerAllowedWrites optional, so AWS may leave it out when it is false, its default.
		"AllowWrites": aws.ToBool(a.ProducerAllowedWrites),
	}
	if arn := aws.ToString(share.ProducerArn); arn != "" {
		properties["ProducerArn"] = arn
	}
	return properties
}

// Read returns the authorization's properties, or ErrNotFound when the data share isn't authorized for the consumer.
func (h *DataShareAuthorization) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	share, a, err := h.find(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	if a == nil || !slices.Contains(dataShareAuthorizedStatuses, a.Status) {
		return nil, native.ErrNotFound
	}
	return dataShareAuthorizationProperties(share, a), nil
}

// Create authorizes the data share for the consumer, and waits until a read shows it. The producer must have granted
// it to the consumer in SQL first.
func (h *DataShareAuthorization) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	if identifier == "" {
		return "", dataShareInvalid(dataShareAuthorizationType, "DataShareArn and ConsumerIdentifier", "set")
	}
	arn, consumer := awsapi.String(request.Properties, "DataShareArn"), awsapi.String(request.Properties, "ConsumerIdentifier")
	allowWrites := dataShareBool(request.Properties, "AllowWrites")
	if err := h.authorize(ctx, request.Region, arn, consumer, allowWrites); err != nil {
		return identifier, err
	}
	return identifier, h.confirm(ctx, request.Region, identifier, allowWrites)
}

// confirm waits until a read shows the authorization with allowWrites.
func (h *DataShareAuthorization) confirm(ctx context.Context, region string, identifier string, allowWrites bool) error {
	return dataShareConfirm(ctx, "the authorization "+identifier, func() (bool, error) {
		_, a, err := h.find(ctx, region, identifier)
		if err != nil || a == nil {
			return false, err
		}
		return slices.Contains(dataShareAuthorizedStatuses, a.Status) && aws.ToBool(a.ProducerAllowedWrites) == allowWrites, nil
	})
}

// authorize sends AuthorizeDataShare with allowWrites. AWS sets AllowWrites to false when the call leaves it out, so
// the handler always sends it.
func (h *DataShareAuthorization) authorize(ctx context.Context, region string, arn string, consumer string, allowWrites bool) error {
	_, err := h.api.AuthorizeDataShare(ctx, &redshift.AuthorizeDataShareInput{
		DataShareArn:       aws.String(arn),
		ConsumerIdentifier: aws.String(consumer),
		AllowWrites:        aws.Bool(allowWrites),
	}, dataShareInRegion(region))
	if awsapi.HasCode(err, dataShareFault) {
		return &native.Failure{
			Message: "AWS has no data share with the ARN granted to the consumer. Create the data share and grant it to the consumer in SQL first, " +
				"such as GRANT USAGE ON DATASHARE <name> TO ACCOUNT '<account ID>', then deploy again: " + err.Error(),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return awsapi.Rejected(err)
}

// Update changes AllowWrites by authorizing the data share again, which keeps the consumer's associations. It first
// reads the data share, so it never changes one Redshift made for itself. A template
// that no longer sets AllowWrites sets it back to false, AWS's default.
func (h *DataShareAuthorization) Update(ctx context.Context, request native.UpdateRequest) error {
	if _, err := h.Identifier(request.Desired); err != nil {
		return err
	}
	if !slices.Contains(awsapi.Changed(request), "AllowWrites") {
		return nil
	}
	arn, consumer, err := h.split(request.Identifier)
	if err != nil {
		return err
	}
	if _, _, err := h.find(ctx, request.Region, request.Identifier); err != nil {
		return err
	}
	allowWrites := dataShareBool(request.Desired, "AllowWrites")
	if err := h.authorize(ctx, request.Region, arn, consumer, allowWrites); err != nil {
		return err
	}
	return h.confirm(ctx, request.Region, request.Identifier, allowWrites)
}

// Delete removes the authorization. AWS then also removes the consumer's associations with the data share. It first
// reads the data share, so it never removes the authorization of one Redshift made for itself, and sends nothing for
// an authorization that is already gone.
func (h *DataShareAuthorization) Delete(ctx context.Context, region string, identifier string) error {
	arn, consumer, err := h.split(identifier)
	if err != nil {
		return err
	}
	_, a, err := h.find(ctx, region, identifier)
	if err != nil {
		return err
	}
	if a == nil || !slices.Contains(dataShareAuthorizedStatuses, a.Status) {
		return native.ErrNotFound
	}
	_, err = h.api.DeauthorizeDataShare(ctx, &redshift.DeauthorizeDataShareInput{
		DataShareArn:       aws.String(arn),
		ConsumerIdentifier: aws.String(consumer),
	}, dataShareInRegion(region))
	return dataShareGone(err, func() bool {
		_, readErr := h.Read(ctx, region, identifier)
		return errors.Is(readErr, native.ErrNotFound)
	})
}

// List returns the authorization of every data share the account produces for each consumer.
func (h *DataShareAuthorization) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	pages := redshift.NewDescribeDataSharesForProducerPaginator(h.api, &redshift.DescribeDataSharesForProducerInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, dataShareInRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.DataShares {
			share := &page.DataShares[i]
			if share.DataShareType != "" {
				continue
			}
			for j := range share.DataShareAssociations {
				a := &share.DataShareAssociations[j]
				if aws.ToString(a.ConsumerRegion) != "" || !slices.Contains(dataShareAuthorizedStatuses, a.Status) {
					continue
				}
				out = append(out, native.Resource{
					Identifier: aws.ToString(share.DataShareArn) + "|" + aws.ToString(a.ConsumerIdentifier),
					Properties: dataShareAuthorizationProperties(share, a),
				})
			}
		}
	}
	return out, nil
}

// Progress reads the authorization. A create or an update is done once the data share is AUTHORIZED, or ACTIVE when
// the consumer has associated it, and never waits for the consumer. AWS authorizes at once, and a grant it hasn't
// authorized reads as PENDING_AUTHORIZATION whether or not a create was sent, so a create that doesn't find the
// authorization fails, and the next deploy authorizes again. A delete is done once the authorization is gone.
func (h *DataShareAuthorization) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	_, a, err := h.find(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if a == nil || !slices.Contains(dataShareAuthorizedStatuses, a.Status) {
		return native.Progress{}, native.ErrNotFound
	}
	if op == native.OperationDelete {
		return native.Progress{State: string(a.Status), Waiting: string(types.DataShareStatusDeauthorized)}, nil
	}
	return native.Progress{Done: true, State: string(a.Status)}, nil
}
