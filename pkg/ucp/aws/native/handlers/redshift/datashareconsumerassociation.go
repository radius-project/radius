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

const (
	dataShareConsumerType = "AWS::Redshift::DataShareConsumerAssociation"

	// dataShareEntireAccount is the last part of the identifier of an association with the entire account, which has
	// no ARN or Region of its own.
	dataShareEntireAccount = "entire-account"
)

var (
	_ native.Handler           = (*DataShareConsumerAssociation)(nil)
	_ native.IdentifierEncoder = (*DataShareConsumerAssociation)(nil)

	// dataShareNamespaceArn is the form of a consumer namespace's ARN: a provisioned cluster's namespace, or a
	// Redshift Serverless one.
	dataShareNamespaceArn = regexp.MustCompile(`^arn:aws[a-z-]*:(redshift:[a-z0-9-]+:[^:|\s]+:namespace:[^:/|\s]+|redshift-serverless:[a-z0-9-]+:[^:|\s]+:namespace/[^:/|\s]+)$`)

	// dataShareRegion is the form of a Region, such as us-west-2.
	dataShareRegion = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]+$`)

	// dataShareAccount is the form of the consumer identifier of an association with the entire account.
	dataShareAccount = regexp.MustCompile(`^[0-9]{12}$`)
)

// DataShareConsumerAssociation serves AWS::Redshift::DataShareConsumerAssociation: a consumer account's association
// of a data share another account authorized for it, with the entire account, one namespace, or every namespace of a
// Region. Its identifier is DataShareArn, "|", and ConsumerArn, ConsumerRegion, or entire-account.
//
// AWS replaces an association with the entire account with one with a namespace or a Region, and the other way around.
// Another alias, or someone outside Radius, may own the association AWS would replace, so a create that would replace
// one fails and changes nothing.
type DataShareConsumerAssociation struct {
	api DataShareAPI
}

// NewDataShareConsumerAssociation returns the handler for Redshift data share consumer associations.
func NewDataShareConsumerAssociation(api DataShareAPI) *DataShareConsumerAssociation {
	return &DataShareConsumerAssociation{api: api}
}

// TypeName returns AWS::Redshift::DataShareConsumerAssociation.
func (h *DataShareConsumerAssociation) TypeName() string {
	return dataShareConsumerType
}

// Options returns how UCP uses the handler. Associating again is safe: AWS answers with the same association.
func (h *DataShareConsumerAssociation) Options() native.Options {
	return native.Options{List: native.ListRegion, Recovery: native.RecoverByIdentifier}
}

// dataShareConsumer is what a consumer association associates a data share with: exactly one of the entire account, a
// namespace, or a Region.
type dataShareConsumer struct {
	entireAccount bool
	arn           string
	region        string
}

// String returns the last part of the association's identifier.
func (c dataShareConsumer) String() string {
	switch {
	case c.arn != "":
		return c.arn
	case c.region != "":
		return c.region
	}
	return dataShareEntireAccount
}

// matches reports whether a is AWS's association with c, as the consumer account reports it: an association with a
// namespace has the namespace's ARN as its consumer identifier, and one with a Region or the entire account has the
// account ID, with the Region or none.
func (c dataShareConsumer) matches(a *types.DataShareAssociation) bool {
	id, region := aws.ToString(a.ConsumerIdentifier), aws.ToString(a.ConsumerRegion)
	switch {
	case c.arn != "":
		return id == c.arn
	case c.region != "":
		return dataShareAccount.MatchString(id) && region == c.region
	}
	return dataShareAccount.MatchString(id) && region == ""
}

// dataShareConsumerOf returns the consumer of an association AWS reports.
func dataShareConsumerOf(a *types.DataShareAssociation) (dataShareConsumer, bool) {
	id, region := aws.ToString(a.ConsumerIdentifier), aws.ToString(a.ConsumerRegion)
	switch {
	case dataShareNamespaceArn.MatchString(id):
		return dataShareConsumer{arn: id}, true
	case dataShareAccount.MatchString(id) && region != "":
		return dataShareConsumer{region: region}, true
	case dataShareAccount.MatchString(id):
		return dataShareConsumer{entireAccount: true}, true
	}
	return dataShareConsumer{}, false
}

// Identifier checks the template's values, so UCP rejects one AWS wouldn't take before any AWS call, and returns
// DataShareArn and the consumer joined with "|", or "" when the template leaves out DataShareArn.
func (h *DataShareConsumerAssociation) Identifier(properties map[string]any) (string, error) {
	arn, c, err := h.parse(properties)
	if err != nil || arn == "" {
		return "", err
	}
	return arn + "|" + c.String(), nil
}

// parse checks the template's values and returns its data share ARN and consumer.
func (h *DataShareConsumerAssociation) parse(properties map[string]any) (string, dataShareConsumer, error) {
	if err := dataShareCheckArn(dataShareConsumerType, properties); err != nil {
		return "", dataShareConsumer{}, err
	}
	if err := dataShareCheckBools(dataShareConsumerType, properties, "AssociateEntireAccount", "AllowWrites"); err != nil {
		return "", dataShareConsumer{}, err
	}
	for _, check := range []struct {
		name    string
		pattern *regexp.Regexp
		want    string
	}{
		{"ConsumerArn", dataShareNamespaceArn, "the ARN of a Redshift namespace"},
		{"ConsumerRegion", dataShareRegion, "a Region, such as us-west-2"},
	} {
		if value, ok := properties[check.name]; ok && value != nil {
			if s, ok := value.(string); !ok || !check.pattern.MatchString(s) {
				return "", dataShareConsumer{}, dataShareInvalid(dataShareConsumerType, check.name, check.want)
			}
		}
	}
	c := dataShareConsumer{
		entireAccount: dataShareBool(properties, "AssociateEntireAccount"),
		arn:           awsapi.String(properties, "ConsumerArn"),
		region:        awsapi.String(properties, "ConsumerRegion"),
	}
	set := 0
	for _, ok := range []bool{c.entireAccount, c.arn != "", c.region != ""} {
		if ok {
			set++
		}
	}
	arn := awsapi.String(properties, "DataShareArn")
	if set != 1 && (arn != "" || set > 1) {
		return "", dataShareConsumer{}, &native.Failure{
			Message:     fmt.Sprintf("an %s must set exactly one of AssociateEntireAccount (true), ConsumerArn, or ConsumerRegion", dataShareConsumerType),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return arn, c, nil
}

// split returns the data share ARN and the consumer of an identifier.
func (h *DataShareConsumerAssociation) split(identifier string) (string, dataShareConsumer, error) {
	arn, last, _ := strings.Cut(identifier, "|")
	properties := map[string]any{"DataShareArn": arn}
	switch {
	case last == dataShareEntireAccount:
		properties["AssociateEntireAccount"] = true
	case strings.HasPrefix(last, "arn:"):
		properties["ConsumerArn"] = last
	default:
		properties["ConsumerRegion"] = last
	}
	parsed, c, err := h.parse(properties)
	if err != nil || parsed == "" {
		return "", dataShareConsumer{}, &native.Failure{
			Message:     fmt.Sprintf("the identifier %q of an %s must be DataShareArn|ConsumerArn, DataShareArn|ConsumerRegion, or DataShareArn|%s", identifier, dataShareConsumerType, dataShareEntireAccount),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return arn, c, nil
}

// find returns the data share with the ARN as the consumer account sees it, or ErrNotFound when no producer
// authorized it for the account. It reads the data shares of the consumer account only, so a producer account never
// takes its own authorization for a consumer's association.
func (h *DataShareConsumerAssociation) find(ctx context.Context, region string, arn string, identifier string) (*types.DataShare, error) {
	pages := redshift.NewDescribeDataSharesForConsumerPaginator(h.api, &redshift.DescribeDataSharesForConsumerInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, dataShareInRegion(region))
		if err != nil {
			return nil, err
		}
		if i := slices.IndexFunc(page.DataShares, func(s types.DataShare) bool { return aws.ToString(s.DataShareArn) == arn }); i >= 0 {
			return dataShareOwn(&page.DataShares[i], identifier)
		}
	}
	return nil, native.ErrNotFound
}

// association returns the data share and the consumer of the identifier, and AWS's association with the consumer,
// which is nil when the consumer has none.
func (h *DataShareConsumerAssociation) association(ctx context.Context, region string, identifier string) (*types.DataShare, dataShareConsumer, *types.DataShareAssociation, error) {
	arn, c, err := h.split(identifier)
	if err != nil {
		return nil, c, nil, err
	}
	share, err := h.find(ctx, region, arn, identifier)
	if err != nil {
		return nil, c, nil, err
	}
	for i := range share.DataShareAssociations {
		if c.matches(&share.DataShareAssociations[i]) {
			return share, c, &share.DataShareAssociations[i], nil
		}
	}
	return share, c, nil, nil
}

// dataShareConsumerProperties returns the properties of the association a of share with c.
func dataShareConsumerProperties(share *types.DataShare, c dataShareConsumer, a *types.DataShareAssociation) map[string]any {
	properties := map[string]any{
		"DataShareArn":           aws.ToString(share.DataShareArn),
		"AssociateEntireAccount": c.entireAccount,
		// The API reference marks ConsumerAcceptedWrites optional, so AWS may leave it out when it is false, its default.
		"AllowWrites": aws.ToBool(a.ConsumerAcceptedWrites),
	}
	if c.arn != "" {
		properties["ConsumerArn"] = c.arn
	}
	if c.region != "" {
		properties["ConsumerRegion"] = c.region
	}
	if arn := aws.ToString(share.ProducerArn); arn != "" {
		properties["ProducerArn"] = arn
	}
	return properties
}

// Read returns the association's properties, or ErrNotFound when it isn't ACTIVE. The consumer account reports a data
// share authorized for it, but not associated, as AVAILABLE.
func (h *DataShareConsumerAssociation) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	share, c, a, err := h.association(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	if a == nil || a.Status != types.DataShareStatusActive {
		return nil, native.ErrNotFound
	}
	return dataShareConsumerProperties(share, c, a), nil
}

// Create associates the data share with the consumer, unless AWS would replace another association of the data
// share in the account.
func (h *DataShareConsumerAssociation) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	arn, c, err := h.parse(request.Properties)
	if err != nil {
		return "", err
	}
	if arn == "" {
		return "", dataShareInvalid(dataShareConsumerType, "DataShareArn", "set")
	}
	identifier := arn + "|" + c.String()
	share, err := h.find(ctx, request.Region, arn, identifier)
	if errors.Is(err, native.ErrNotFound) {
		return "", dataShareNotAuthorized(nil)
	} else if err != nil {
		return "", err
	}
	if err := dataShareReplaced(share, c); err != nil {
		return "", err
	}
	allowWrites := dataShareBool(request.Properties, "AllowWrites")
	if err := h.associate(ctx, request.Region, arn, c, allowWrites); err != nil {
		return identifier, err
	}
	return identifier, h.confirm(ctx, request.Region, identifier, allowWrites)
}

// confirm waits until a read shows the association ACTIVE with allowWrites.
func (h *DataShareConsumerAssociation) confirm(ctx context.Context, region string, identifier string, allowWrites bool) error {
	return dataShareConfirm(ctx, "the association "+identifier, func() (bool, error) {
		_, _, a, err := h.association(ctx, region, identifier)
		if err != nil || a == nil {
			return false, err
		}
		return a.Status == types.DataShareStatusActive && aws.ToBool(a.ConsumerAcceptedWrites) == allowWrites, nil
	})
}

// dataShareReplaced returns the failure of an association with c that AWS would make by replacing another ACTIVE association
// of share: one with the entire account replaces those with a namespace or a Region, and one with a namespace or a
// Region replaces the one with the entire account.
func dataShareReplaced(share *types.DataShare, c dataShareConsumer) error {
	others := 0
	for i := range share.DataShareAssociations {
		a := &share.DataShareAssociations[i]
		other, ok := dataShareConsumerOf(a)
		if ok && a.Status == types.DataShareStatusActive && other.entireAccount != c.entireAccount {
			others++
		}
	}
	if others == 0 {
		return nil
	}
	what := "its association with the entire account"
	if c.entireAccount {
		what = fmt.Sprintf("its %d associations with a namespace or a Region", others)
	}
	return &native.Failure{
		Message: fmt.Sprintf("AWS would replace %s with this one, and another alias or someone outside Radius may own it, so UCP changed "+
			"nothing. Remove it first, or associate the data share the same way it already is.", what),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// dataShareNotAuthorized is the failure of an association with a data share no producer authorized for the account.
func dataShareNotAuthorized(err error) error {
	message := "AWS has no data share with the ARN authorized for this account. The producer account authorizes it first, such as with " +
		dataShareAuthorizationType + ", then deploy again"
	if err != nil {
		message += ": " + err.Error()
	}
	return &native.Failure{Message: message, ClientError: true, Outcome: native.OutcomeRejected}
}

// associate sends AssociateDataShareConsumer for c with allowWrites. AWS sets AllowWrites to false when the call
// leaves it out, so the handler always sends it.
func (h *DataShareConsumerAssociation) associate(ctx context.Context, region string, arn string, c dataShareConsumer, allowWrites bool) error {
	input := &redshift.AssociateDataShareConsumerInput{DataShareArn: aws.String(arn), AllowWrites: aws.Bool(allowWrites)}
	switch {
	case c.arn != "":
		input.ConsumerArn = aws.String(c.arn)
	case c.region != "":
		input.ConsumerRegion = aws.String(c.region)
	default:
		input.AssociateEntireAccount = aws.Bool(true)
	}
	_, err := h.api.AssociateDataShareConsumer(ctx, input, dataShareInRegion(region))
	if awsapi.HasCode(err, dataShareFault) {
		return dataShareNotAuthorized(err)
	}
	return awsapi.Rejected(err)
}

// Update changes AllowWrites by associating the data share again the same way, which keeps the association. It first
// reads the association, so it never changes one with a data share Redshift made for itself, and never brings back
// one that is gone. A
// template that no longer sets AllowWrites sets it back to false, AWS's default.
func (h *DataShareConsumerAssociation) Update(ctx context.Context, request native.UpdateRequest) error {
	if _, err := h.Identifier(request.Desired); err != nil {
		return err
	}
	if !slices.Contains(awsapi.Changed(request), "AllowWrites") {
		return nil
	}
	_, c, a, err := h.association(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	if a == nil || a.Status != types.DataShareStatusActive {
		// Another client replaced or removed the association after UCP read it. Associating again would replace theirs.
		return native.ErrNotFound
	}
	arn, _, _ := strings.Cut(request.Identifier, "|")
	allowWrites := dataShareBool(request.Desired, "AllowWrites")
	if err := h.associate(ctx, request.Region, arn, c, allowWrites); err != nil {
		return err
	}
	return h.confirm(ctx, request.Region, request.Identifier, allowWrites)
}

// Delete removes the association. It first reads the data share, so it never removes an association with one
// Redshift made for itself, and sends nothing for an association that is already gone.
func (h *DataShareConsumerAssociation) Delete(ctx context.Context, region string, identifier string) error {
	arn, c, err := h.split(identifier)
	if err != nil {
		return err
	}
	_, _, a, err := h.association(ctx, region, identifier)
	if err != nil {
		return err
	}
	if a == nil || a.Status != types.DataShareStatusActive {
		return native.ErrNotFound
	}
	input := &redshift.DisassociateDataShareConsumerInput{DataShareArn: aws.String(arn)}
	switch {
	case c.arn != "":
		input.ConsumerArn = aws.String(c.arn)
	case c.region != "":
		input.ConsumerRegion = aws.String(c.region)
	default:
		input.DisassociateEntireAccount = aws.Bool(true)
	}
	_, err = h.api.DisassociateDataShareConsumer(ctx, input, dataShareInRegion(region))
	return dataShareGone(err, func() bool {
		_, readErr := h.Read(ctx, region, identifier)
		return errors.Is(readErr, native.ErrNotFound)
	})
}

// List returns every ACTIVE association of the data shares producers authorized for the account.
func (h *DataShareConsumerAssociation) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	pages := redshift.NewDescribeDataSharesForConsumerPaginator(h.api, &redshift.DescribeDataSharesForConsumerInput{})
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
				c, ok := dataShareConsumerOf(a)
				if !ok || a.Status != types.DataShareStatusActive {
					continue
				}
				out = append(out, native.Resource{
					Identifier: aws.ToString(share.DataShareArn) + "|" + c.String(),
					Properties: dataShareConsumerProperties(share, c, a),
				})
			}
		}
	}
	return out, nil
}

// Progress reads the association. A create or an update is done once it is ACTIVE. AWS associates at once, and an
// association that isn't ACTIVE reads the same whether or not a create was sent, so a create that doesn't find it
// fails, and the next deploy associates again. A delete is done once it is gone.
func (h *DataShareConsumerAssociation) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	_, _, a, err := h.association(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if a == nil || a.Status != types.DataShareStatusActive {
		return native.Progress{}, native.ErrNotFound
	}
	if op == native.OperationDelete {
		return native.Progress{State: string(a.Status), Waiting: "disassociated"}, nil
	}
	return native.Progress{Done: true, State: string(a.Status)}, nil
}
