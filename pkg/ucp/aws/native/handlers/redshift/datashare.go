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
	"fmt"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

// DataShareAPI is the part of the Redshift client the data share handlers use. A data share authorization is the
// producer's side, and a consumer association the consumer account's side, so each handler calls only its own side's
// operations.
type DataShareAPI interface {
	AuthorizeDataShare(ctx context.Context, params *redshift.AuthorizeDataShareInput, optFns ...func(*redshift.Options)) (*redshift.AuthorizeDataShareOutput, error)
	DeauthorizeDataShare(ctx context.Context, params *redshift.DeauthorizeDataShareInput, optFns ...func(*redshift.Options)) (*redshift.DeauthorizeDataShareOutput, error)
	DescribeDataShares(ctx context.Context, params *redshift.DescribeDataSharesInput, optFns ...func(*redshift.Options)) (*redshift.DescribeDataSharesOutput, error)
	DescribeDataSharesForProducer(ctx context.Context, params *redshift.DescribeDataSharesForProducerInput, optFns ...func(*redshift.Options)) (*redshift.DescribeDataSharesForProducerOutput, error)

	AssociateDataShareConsumer(ctx context.Context, params *redshift.AssociateDataShareConsumerInput, optFns ...func(*redshift.Options)) (*redshift.AssociateDataShareConsumerOutput, error)
	DisassociateDataShareConsumer(ctx context.Context, params *redshift.DisassociateDataShareConsumerInput, optFns ...func(*redshift.Options)) (*redshift.DisassociateDataShareConsumerOutput, error)
	DescribeDataSharesForConsumer(ctx context.Context, params *redshift.DescribeDataSharesForConsumerInput, optFns ...func(*redshift.Options)) (*redshift.DescribeDataSharesForConsumerOutput, error)
}

var dataShareInRegion = awsapi.InRegion[redshift.Options]

const (
	// dataShareFault is the code Redshift returns for a data share it doesn't have, or one the call can't act on, such
	// as an authorization it already removed. dataShareNamespaceFault is the code of a consumer namespace it doesn't
	// have.
	dataShareFault          = "InvalidDataShareFault"
	dataShareNamespaceFault = "InvalidNamespaceFault"

	// dataShareParameterFault is the code of a request whose values Redshift rejects, which the SDK doesn't model.
	dataShareParameterFault = "InvalidParameterValue"
)

// dataShareArn is the form of a data share's ARN: the producer's namespace ID and the data share's name after
// "datashare:". It checks only that form, so the identifier splits at its first "|", and leaves the rest to AWS.
var dataShareArn = regexp.MustCompile(`^arn:aws[a-z-]*:redshift:[a-z0-9-]+:[^:|\s]+:datashare:[^/|\s]+/[^/|\s]+$`)

// dataShareInvalid is the failure of a template value the handler can't send. It names the property and what it must
// be, never the value.
func dataShareInvalid(typeName string, name string, want string) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s of an %s must be %s", name, typeName, want),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// dataShareCheckArn checks that DataShareArn, when the template sets it, is a data share's ARN.
func dataShareCheckArn(typeName string, properties map[string]any) error {
	value, ok := properties["DataShareArn"]
	if !ok || value == nil {
		return nil
	}
	if s, ok := value.(string); !ok || !dataShareArn.MatchString(s) {
		return dataShareInvalid(typeName, "DataShareArn", "the ARN of a data share, such as arn:aws:redshift:us-west-2:123456789012:datashare:<namespace ID>/<name>")
	}
	return nil
}

// dataShareCheckBools checks that each named property the template sets is a boolean.
func dataShareCheckBools(typeName string, properties map[string]any, names ...string) error {
	for _, name := range names {
		if value, ok := properties[name]; ok && value != nil {
			if _, ok := value.(bool); !ok {
				return dataShareInvalid(typeName, name, "true or false")
			}
		}
	}
	return nil
}

// dataShareBool returns the boolean property name of properties, or false when it isn't set.
func dataShareBool(properties map[string]any, name string) bool {
	b, _ := properties[name].(bool)
	return b
}

// dataShareOwn returns share, or an OtherProductError when Redshift made it for itself, such as the INTERNAL data
// share that registering a namespace with the AWS Glue Data Catalog makes. That share and its authorization belong to
// the namespace registration, so these handlers never read, change, or remove them.
func dataShareOwn(share *types.DataShare, identifier string) (*types.DataShare, error) {
	if share.DataShareType != "" {
		return nil, &native.OtherProductError{Identifier: identifier, Product: "a data share of type " + string(share.DataShareType)}
	}
	return share, nil
}

// dataShareGone returns a failed call's error as ErrNotFound when AWS answered InvalidDataShareFault and gone reports
// that AWS no longer has what the call acts on, such as an authorization someone already removed. Redshift uses that
// code for other rejections too, so the handler reads again rather than take the code alone as not found.
func dataShareGone(err error, gone func() bool) error {
	if awsapi.HasCode(err, dataShareFault) && gone() {
		return fmt.Errorf("%w: %w", native.ErrNotFound, err)
	}
	return awsapi.Rejected(err)
}
