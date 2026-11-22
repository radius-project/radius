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
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const partnerType = "AWS::Redshift::PartnerIntegration"

// PartnerAPI is the part of the Redshift client PartnerIntegration uses.
type PartnerAPI interface {
	AddPartner(ctx context.Context, params *redshift.AddPartnerInput, optFns ...func(*redshift.Options)) (*redshift.AddPartnerOutput, error)
	DeletePartner(ctx context.Context, params *redshift.DeletePartnerInput, optFns ...func(*redshift.Options)) (*redshift.DeletePartnerOutput, error)
	DescribePartners(ctx context.Context, params *redshift.DescribePartnersInput, optFns ...func(*redshift.Options)) (*redshift.DescribePartnersOutput, error)
}

var (
	_ native.Handler           = (*PartnerIntegration)(nil)
	_ native.IdentifierEncoder = (*PartnerIntegration)(nil)

	partnerRegion = awsapi.InRegion[redshift.Options]

	// partnerFields are the forms of the values that identify a partner integration, from the AWS API reference. A
	// cluster identifier is in lowercase, as AWS keeps it, so one cluster has one identifier.
	partnerFields = []struct {
		name    string
		pattern *regexp.Regexp
		want    string
	}{
		{"AccountId", regexp.MustCompile(`^[0-9]{12}$`), "an AWS account ID of 12 digits"},
		{"ClusterIdentifier", regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`), "a lowercase letter, then up to 62 lowercase letters, digits, and hyphens"},
		{"DatabaseName", regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}@$#_]{1,126}$`), "2 to 127 letters, digits, and @$#_, starting with a letter or _"},
		{"PartnerName", regexp.MustCompile(`^[a-zA-Z0-9\-_]{1,255}$`), "1 to 255 letters, digits, hyphens, and underscores"},
	}
)

const (
	// partnerClusterNotFound is the code of a cluster AWS doesn't have, and partnerNotFound of a partner integration
	// or a partner it doesn't have.
	partnerClusterNotFound = "ClusterNotFound"
	partnerNotFound        = "PartnerNotFound"
)

// PartnerIntegration serves AWS::Redshift::PartnerIntegration: a partner allowed to send status updates for a
// database of a provisioned cluster, made with AddPartner. Its identifier is AccountId, ClusterIdentifier,
// DatabaseName, and PartnerName joined with "|". The partner sets Status and StatusMessage with UpdatePartnerStatus,
// so they are read-only and the handler never sets them.
type PartnerIntegration struct {
	api PartnerAPI
}

// NewPartnerIntegration returns the handler for Redshift partner integrations.
func NewPartnerIntegration(api PartnerAPI) *PartnerIntegration {
	return &PartnerIntegration{api: api}
}

// TypeName returns AWS::Redshift::PartnerIntegration.
func (h *PartnerIntegration) TypeName() string {
	return partnerType
}

// Options returns how UCP uses the handler. AWS accepts an integration it already has, so UCP sends a lost create
// again when it can't find it.
func (h *PartnerIntegration) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// partnerValues returns the identifying values properties give, in the order of partnerFields, and whether they give
// all of them. A value it can't use is a client failure that names the property and what it must be, never the value.
func partnerValues(properties map[string]any) ([]string, bool, error) {
	values := []string{}
	complete := true
	for _, field := range partnerFields {
		value, ok := properties[field.name]
		if !ok || value == nil {
			complete = false
			values = append(values, "")
			continue
		}
		s, ok := value.(string)
		if !ok || !field.pattern.MatchString(s) {
			return nil, false, &native.Failure{
				Message:     fmt.Sprintf("%s of an %s must be %s", field.name, partnerType, field.want),
				ClientError: true,
				Outcome:     native.OutcomeRejected,
			}
		}
		values = append(values, s)
	}
	return values, complete, nil
}

// Identifier checks the template's values, so UCP rejects one AWS wouldn't take before any AWS call, and returns
// AccountId, ClusterIdentifier, DatabaseName, and PartnerName joined with "|".
func (h *PartnerIntegration) Identifier(properties map[string]any) (string, error) {
	values, complete, err := partnerValues(properties)
	if err != nil || !complete {
		return "", err
	}
	return strings.Join(values, "|"), nil
}

// partner is the values that identify a partner integration.
type partner struct {
	account, cluster, database, name string
}

// splitPartner returns the values of an identifier.
func splitPartner(identifier string) (partner, error) {
	parts := strings.Split(identifier, "|")
	properties := map[string]any{}
	for i, field := range partnerFields {
		if i < len(parts) {
			properties[field.name] = parts[i]
		}
	}
	_, complete, err := partnerValues(properties)
	if len(parts) != len(partnerFields) || err != nil || !complete {
		return partner{}, &native.Failure{
			Message:     fmt.Sprintf("the identifier %q of an %s must be AccountId|ClusterIdentifier|DatabaseName|PartnerName", identifier, partnerType),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return partner{account: parts[0], cluster: parts[1], database: parts[2], name: parts[3]}, nil
}

// get returns the partner integration, or ErrNotFound.
func (h *PartnerIntegration) get(ctx context.Context, region string, p partner) (*types.PartnerIntegrationInfo, error) {
	out, err := h.api.DescribePartners(ctx, &redshift.DescribePartnersInput{
		AccountId:         aws.String(p.account),
		ClusterIdentifier: aws.String(p.cluster),
		DatabaseName:      aws.String(p.database),
		PartnerName:       aws.String(p.name),
	}, partnerRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, partnerClusterNotFound)
	}
	for i := range out.PartnerIntegrationInfoList {
		info := &out.PartnerIntegrationInfoList[i]
		if aws.ToString(info.DatabaseName) == p.database && aws.ToString(info.PartnerName) == p.name {
			return info, nil
		}
	}
	return nil, native.ErrNotFound
}

// Read returns the partner integration's properties.
func (h *PartnerIntegration) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	p, err := splitPartner(identifier)
	if err != nil {
		return nil, err
	}
	info, err := h.get(ctx, region, p)
	if err != nil {
		return nil, err
	}
	properties := map[string]any{
		"AccountId":         p.account,
		"ClusterIdentifier": p.cluster,
		"DatabaseName":      p.database,
		"PartnerName":       p.name,
	}
	if info.Status != "" {
		properties["Status"] = string(info.Status)
	}
	if message := aws.ToString(info.StatusMessage); message != "" {
		properties["StatusMessage"] = message
	}
	return properties, nil
}

// Create adds the partner integration.
func (h *PartnerIntegration) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	values, complete, err := partnerValues(request.Properties)
	if err != nil {
		return "", err
	}
	if !complete {
		return "", &native.Failure{
			Message:     fmt.Sprintf("AccountId, ClusterIdentifier, DatabaseName, and PartnerName of an %s must be set", partnerType),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	_, err = h.api.AddPartner(ctx, &redshift.AddPartnerInput{
		AccountId:         aws.String(values[0]),
		ClusterIdentifier: aws.String(values[1]),
		DatabaseName:      aws.String(values[2]),
		PartnerName:       aws.String(values[3]),
	}, partnerRegion(request.Region))
	if err = awsapi.Rejected(err); err != nil {
		// AWS may have added the integration when it didn't reject the call, such as when the response was lost.
		var failure *native.Failure
		if errors.As(err, &failure) && failure.Outcome == native.OutcomeRejected {
			return "", err
		}
		return strings.Join(values, "|"), err
	}
	return strings.Join(values, "|"), nil
}

// Update changes nothing: every property the template sets identifies the integration, so UCP rejects a change to
// any of them before it calls the handler, and the partner sets the others.
func (h *PartnerIntegration) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete deletes the partner integration. Data can still flow to the cluster until the integration is also deleted
// at the partner's website.
func (h *PartnerIntegration) Delete(ctx context.Context, region string, identifier string) error {
	p, err := splitPartner(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeletePartner(ctx, &redshift.DeletePartnerInput{
		AccountId:         aws.String(p.account),
		ClusterIdentifier: aws.String(p.cluster),
		DatabaseName:      aws.String(p.database),
		PartnerName:       aws.String(p.name),
	}, partnerRegion(region))
	return awsapi.Rejected(awsapi.NotFound(err, partnerNotFound, partnerClusterNotFound))
}

// List isn't supported: AWS lists partner integrations only per cluster.
func (h *PartnerIntegration) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("%s can't be listed", partnerType)
}

// Progress reads the partner integration. A create is done once AWS lists the integration, and a delete waits until
// AWS no longer does. AWS lists a new integration at once, so one it doesn't list is gone, and the next deploy adds it.
func (h *PartnerIntegration) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	p, err := splitPartner(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if _, err := h.get(ctx, region, p); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{State: "listed", Waiting: "deleted"}, nil
	}
	return native.Progress{Done: true, State: "listed"}, nil
}
