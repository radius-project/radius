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

package redshiftserverless

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const customDomainType = "AWS::RedshiftServerless::CustomDomainAssociation"

var (
	_ native.Handler           = (*CustomDomainAssociation)(nil)
	_ native.IdentifierEncoder = (*CustomDomainAssociation)(nil)

	// workgroupName and domainName are the forms AWS takes, with the domain in lowercase, so one domain has one
	// identifier.
	workgroupName = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)
	domainName    = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$`)
)

const maxDomainName = 253

// CustomDomainAssociation serves AWS::RedshiftServerless::CustomDomainAssociation. Its identifier is WorkgroupName
// and CustomDomainName joined with "|", which the template gives. A change of the association changes the workgroup,
// so each operation waits for the workgroup to be free before it finishes.
type CustomDomainAssociation struct {
	api API
}

// NewCustomDomainAssociation returns the handler for Redshift Serverless custom domain associations.
func NewCustomDomainAssociation(api API) *CustomDomainAssociation {
	return &CustomDomainAssociation{api: api}
}

// TypeName returns AWS::RedshiftServerless::CustomDomainAssociation.
func (h *CustomDomainAssociation) TypeName() string {
	return customDomainType
}

// Options returns how UCP uses the handler.
func (h *CustomDomainAssociation) Options() native.Options {
	return native.Options{List: native.ListRegion, Recovery: native.RecoverByIdentifier}
}

// Identifier checks the template's values, so UCP rejects one AWS wouldn't take before any AWS call, and returns
// WorkgroupName and CustomDomainName joined with "|".
func (h *CustomDomainAssociation) Identifier(properties map[string]any) (string, error) {
	if err := checkStrings(customDomainType, properties, "WorkgroupName", "CustomDomainName", "CustomDomainCertificateArn"); err != nil {
		return "", err
	}
	workgroup, domain := awsapi.String(properties, "WorkgroupName"), awsapi.String(properties, "CustomDomainName")
	if workgroup != "" && !workgroupName.MatchString(workgroup) {
		return "", invalid(customDomainType, "WorkgroupName", "3 to 64 lowercase letters, digits, and hyphens")
	}
	if domain != "" && (len(domain) > maxDomainName || !domainName.MatchString(domain)) {
		return "", invalid(customDomainType, "CustomDomainName", "a domain name in lowercase, of at most 253 characters")
	}
	if workgroup == "" || domain == "" {
		return "", nil
	}
	return workgroup + "|" + domain, nil
}

// splitIdentifier returns the workgroup and domain of an identifier.
func splitIdentifier(identifier string) (string, string, error) {
	workgroup, domain, ok := strings.Cut(identifier, "|")
	if !ok || workgroup == "" || domain == "" {
		return "", "", &native.Failure{
			Message:     fmt.Sprintf("the identifier %q of an %s must be WorkgroupName|CustomDomainName", identifier, customDomainType),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return workgroup, domain, nil
}

// get returns the properties of the association with the identifier, or ErrNotFound.
func (h *CustomDomainAssociation) get(ctx context.Context, region string, identifier string) (map[string]any, error) {
	workgroup, domain, err := splitIdentifier(identifier)
	if err != nil {
		return nil, err
	}
	out, err := h.api.GetCustomDomainAssociation(ctx, &redshiftserverless.GetCustomDomainAssociationInput{
		WorkgroupName:    aws.String(workgroup),
		CustomDomainName: aws.String(domain),
	}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, notFound)
	}
	if aws.ToString(out.CustomDomainName) == "" {
		return nil, native.ErrNotFound
	}
	return associationProperties(aws.ToString(out.WorkgroupName), aws.ToString(out.CustomDomainName), aws.ToString(out.CustomDomainCertificateArn), out.CustomDomainCertificateExpiryTime), nil
}

// associationProperties returns the properties of an association as AWS returns it.
func associationProperties(workgroup string, domain string, certificate string, expiry *time.Time) map[string]any {
	properties := map[string]any{}
	for name, value := range map[string]string{
		"WorkgroupName":              workgroup,
		"CustomDomainName":           domain,
		"CustomDomainCertificateArn": certificate,
	} {
		if value != "" {
			properties[name] = value
		}
	}
	if expiry != nil {
		properties["CustomDomainCertificateExpiryTime"] = expiry.UTC().Format(time.RFC3339)
	}
	return properties
}

// Read returns the association's properties.
func (h *CustomDomainAssociation) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	return h.get(ctx, region, identifier)
}

// Create associates the domain with the workgroup.
func (h *CustomDomainAssociation) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	workgroup, domain, err := splitIdentifier(identifier)
	if err != nil {
		return "", err
	}
	_, err = h.api.CreateCustomDomainAssociation(ctx, &redshiftserverless.CreateCustomDomainAssociationInput{
		WorkgroupName:              aws.String(workgroup),
		CustomDomainName:           aws.String(domain),
		CustomDomainCertificateArn: aws.String(awsapi.String(request.Properties, "CustomDomainCertificateArn")),
	}, inRegion(request.Region))
	if awsapi.HasCode(err, conflict) {
		// UCP reads AWS by the identifier before it creates an association, so one AWS has now may be the one an
		// earlier attempt of this create made, such as when the SDK retried a call whose response was lost. The
		// outcome is then unknown, so UCP keeps the record and the next deploy finds it.
		_, getErr := h.get(ctx, request.Region, identifier)
		switch {
		case getErr == nil:
			return "", &native.Failure{
				Message:     fmt.Sprintf("AWS already associates %s with the workgroup %s, which an earlier attempt of this deploy may have done. Deploy again to manage it: %s", domain, workgroup, err.Error()),
				ClientError: true,
				Outcome:     native.OutcomeUnknown,
			}
		case !errors.Is(getErr, native.ErrNotFound):
			return "", unconfirmed(err, getErr)
		}
	}
	if err != nil {
		return "", rejected("The workgroup "+workgroup, err)
	}
	return identifier, nil
}

// Update changes the domain's certificate.
func (h *CustomDomainAssociation) Update(ctx context.Context, request native.UpdateRequest) error {
	if _, err := h.Identifier(request.Desired); err != nil {
		return err
	}
	if !slices.Contains(awsapi.Changed(request), "CustomDomainCertificateArn") {
		return nil
	}
	workgroup, domain, err := splitIdentifier(request.Identifier)
	if err != nil {
		return err
	}
	certificate := awsapi.String(request.Desired, "CustomDomainCertificateArn")
	_, err = h.api.UpdateCustomDomainAssociation(ctx, &redshiftserverless.UpdateCustomDomainAssociationInput{
		WorkgroupName:              aws.String(workgroup),
		CustomDomainName:           aws.String(domain),
		CustomDomainCertificateArn: aws.String(certificate),
	}, inRegion(request.Region))
	if awsapi.HasCode(err, conflict) {
		// An SDK retry after a lost response may find the workgroup busy with the first attempt, so the update reached
		// AWS when the association already has the certificate.
		current, getErr := h.get(ctx, request.Region, request.Identifier)
		if getErr != nil {
			return unconfirmed(err, getErr)
		}
		if awsapi.String(current, "CustomDomainCertificateArn") == certificate {
			return nil
		}
	}
	return rejected("The workgroup "+workgroup, awsapi.NotFound(err, notFound))
}

// Delete removes the domain from the workgroup.
func (h *CustomDomainAssociation) Delete(ctx context.Context, region string, identifier string) error {
	workgroup, domain, err := splitIdentifier(identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeleteCustomDomainAssociation(ctx, &redshiftserverless.DeleteCustomDomainAssociationInput{
		WorkgroupName:    aws.String(workgroup),
		CustomDomainName: aws.String(domain),
	}, inRegion(region))
	return rejected("The workgroup "+workgroup, awsapi.NotFound(err, notFound))
}

// List returns every custom domain association in the region.
func (h *CustomDomainAssociation) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	pages := redshiftserverless.NewListCustomDomainAssociationsPaginator(h.api, &redshiftserverless.ListCustomDomainAssociationsInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, a := range page.Associations {
			workgroup, domain := aws.ToString(a.WorkgroupName), aws.ToString(a.CustomDomainName)
			out = append(out, native.Resource{
				Identifier: workgroup + "|" + domain,
				Properties: associationProperties(workgroup, domain, aws.ToString(a.CustomDomainCertificateArn), a.CustomDomainCertificateExpiryTime),
			})
		}
	}
	return out, nil
}

// Progress waits for the workgroup to be free, because a change of the association changes the workgroup. A create
// or an update also needs the association, and a delete needs it gone.
func (h *CustomDomainAssociation) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	_, err := h.get(ctx, region, identifier)
	gone := op == native.OperationDelete && errors.Is(err, native.ErrNotFound)
	switch {
	case err != nil && !gone:
		return native.Progress{}, err
	case op == native.OperationDelete && !gone:
		return native.Progress{State: "associated", Waiting: "deleted"}, nil
	}
	workgroup, _, _ := strings.Cut(identifier, "|")
	progress, err := workgroupProgress(ctx, h.api, region, workgroup)
	if err != nil || !progress.Done {
		return progress, err
	}
	if gone {
		return native.Progress{}, native.ErrNotFound
	}
	return progress, nil
}
