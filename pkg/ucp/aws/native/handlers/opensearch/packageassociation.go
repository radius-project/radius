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

package opensearch

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const packageAssociationType = "AWS::OpenSearchService::PackageAssociation"

// associationTimeout is how long UCP waits for an association or dissociation. Associating a plugin runs a blue/green
// deployment of the domain, which can take hours on a large domain.
const associationTimeout = 3 * time.Hour

var (
	_ native.Handler           = (*PackageAssociation)(nil)
	_ native.IdentifierEncoder = (*PackageAssociation)(nil)
)

var (
	// associationDomainPattern is the documented pattern of DomainName, which is 3 to 28 characters.
	associationDomainPattern = regexp.MustCompile(`^[a-z][a-z0-9\-]+$`)

	// keyAccessRolePattern is the documented pattern of KeyAccessRoleArn, which is 20 to 2048 characters.
	keyAccessRolePattern = regexp.MustCompile(`^arn:(aws|aws-cn|aws-us-gov|aws-iso|aws-iso-b):iam::[0-9]+:role/.*$`)
)

// PackageAssociation serves AWS::OpenSearchService::PackageAssociation: a package associated with a domain. Its
// identifier is DomainName and PackageID joined with "|". AWS has no call that changes an association, so every
// property is create-only, and UCP rejects a change with 400 before any AWS call. The identifier is known before the
// create, so a lost create is found by it.
type PackageAssociation struct {
	api PackageAPI
}

// NewPackageAssociation returns the handler for OpenSearch Service package associations.
func NewPackageAssociation(api PackageAPI) *PackageAssociation {
	return &PackageAssociation{api: api}
}

// TypeName returns AWS::OpenSearchService::PackageAssociation.
func (h *PackageAssociation) TypeName() string {
	return packageAssociationType
}

// Options returns how UCP uses the handler. AWS lists associations only by domain or by package, so the type can't be
// listed.
func (h *PackageAssociation) Options() native.Options {
	return native.Options{
		List:          native.ListNone,
		Recovery:      native.RecoverByIdentifier,
		CreateTimeout: associationTimeout,
		DeleteTimeout: associationTimeout,
	}
}

// associationID names an association: a domain and a package.
type associationID struct {
	domain, packageID string
}

func (id associationID) String() string {
	return id.domain + "|" + id.packageID
}

func validDomainName(name string) bool {
	return len(name) >= 3 && len(name) <= 28 && associationDomainPattern.MatchString(name)
}

// parseAssociation returns the association the properties name. ok is false when they don't name all of it.
func parseAssociation(properties map[string]any) (id associationID, ok bool, err error) {
	strs := map[string]string{}
	for _, name := range []string{"DomainName", "PackageID"} {
		value, present := properties[name]
		if !present || value == nil {
			continue
		}
		s, isString := value.(string)
		if !isString {
			return associationID{}, false, invalid(packageAssociationType, "%s must be a string", name)
		}
		strs[name] = s
	}
	id = associationID{domain: strs["DomainName"], packageID: strs["PackageID"]}
	if id.domain != "" && !validDomainName(id.domain) {
		return associationID{}, false, invalid(packageAssociationType, "DomainName must be 3 to 28 lowercase letters, digits, and hyphens, starting with a letter")
	}
	if id.packageID != "" && !packageIDPattern.MatchString(id.packageID) {
		return associationID{}, false, invalid(packageAssociationType, "PackageID must be a package ID, such as F123456789 or pkg-0123abcd")
	}
	return id, id.domain != "" && id.packageID != "", nil
}

// parseAssociationID returns the association an identifier names.
func parseAssociationID(identifier string) (associationID, error) {
	domain, packageID, found := strings.Cut(identifier, "|")
	if !found || !validDomainName(domain) || !packageIDPattern.MatchString(packageID) {
		return associationID{}, invalid(packageAssociationType, "%q is not the identifier of an OpenSearch Service package association: it must be DomainName|PackageID", identifier)
	}
	return associationID{domain: domain, packageID: packageID}, nil
}

// Identifier returns DomainName|PackageID, or "" when the properties don't give both. UCP calls it before any AWS
// call, so a template with a value AWS would reject is rejected here, as is one whose other values are invalid.
func (h *PackageAssociation) Identifier(properties map[string]any) (string, error) {
	id, ok, err := parseAssociation(properties)
	if err != nil || !ok {
		return "", err
	}
	if _, err := associationOptions(properties); err != nil {
		return "", err
	}
	return id.String(), nil
}

// associationOptions returns the template's prerequisites and association configuration as an AssociatePackage
// input without the domain and package, or a client failure for a value AWS would reject.
func associationOptions(properties map[string]any) (*opensearch.AssociatePackageInput, error) {
	prerequisites, ok := stringList(properties, "PrerequisitePackageIDList")
	if !ok {
		return nil, invalid(packageAssociationType, "PrerequisitePackageIDList must be a list of package IDs")
	}
	for _, p := range prerequisites {
		if !packageIDPattern.MatchString(p) {
			return nil, invalid(packageAssociationType, "PrerequisitePackageIDList must be a list of package IDs, such as F123456789 or pkg-0123abcd")
		}
	}
	in := &opensearch.AssociatePackageInput{PrerequisitePackageIDList: prerequisites}
	if value, present := properties["AssociationConfiguration"]; present && value != nil {
		config, isObject := value.(map[string]any)
		in.AssociationConfiguration = &types.PackageAssociationConfiguration{}
		if !isObject || awsapi.Decode(config, in.AssociationConfiguration) != nil {
			return nil, invalid(packageAssociationType, "AssociationConfiguration must be an object with KeyStoreAccessOption")
		}
		if o := in.AssociationConfiguration.KeyStoreAccessOption; o != nil {
			if o.KeyStoreAccessEnabled == nil {
				return nil, invalid(packageAssociationType, "AssociationConfiguration.KeyStoreAccessOption must set KeyStoreAccessEnabled")
			}
			if arn := o.KeyAccessRoleArn; arn != nil && (len(*arn) < 20 || len(*arn) > 2048 || !keyAccessRolePattern.MatchString(*arn)) {
				return nil, invalid(packageAssociationType, "AssociationConfiguration.KeyStoreAccessOption.KeyAccessRoleArn must be the ARN of an IAM role")
			}
		}
	}
	return in, nil
}

// find returns the domain's association with the package, or ErrNotFound when the domain doesn't exist or doesn't
// list it.
func (h *PackageAssociation) find(ctx context.Context, region string, id associationID) (*types.DomainPackageDetails, error) {
	input := &opensearch.ListPackagesForDomainInput{DomainName: aws.String(id.domain)}
	for {
		page, err := h.api.ListPackagesForDomain(ctx, input, inRegion(region))
		if err != nil {
			return nil, awsapi.NotFound(err, "ResourceNotFoundException")
		}
		for i := range page.DomainPackageDetailsList {
			if aws.ToString(page.DomainPackageDetailsList[i].PackageID) == id.packageID {
				return &page.DomainPackageDetailsList[i], nil
			}
		}
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			return nil, native.ErrNotFound
		}
	}
}

// Read returns the association's properties.
func (h *PackageAssociation) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, err := parseAssociationID(identifier)
	if err != nil {
		return nil, err
	}
	d, err := h.find(ctx, region, id)
	if err != nil {
		return nil, err
	}
	return awsapi.EncodeObject(map[string]any{
		"DomainName":                id.domain,
		"PackageID":                 id.packageID,
		"PrerequisitePackageIDList": d.PrerequisitePackageIDList,
		"AssociationConfiguration":  d.AssociationConfiguration,
		"DomainPackageStatus":       string(d.DomainPackageStatus),
		"PackageName":               d.PackageName,
		"PackageType":               string(d.PackageType),
		"PackageVersion":            d.PackageVersion,
		"ReferencePath":             d.ReferencePath,
	})
}

// associationBusy returns the conflict of a change AWS can't make now because it is changing the domain or another of
// its packages. UCP answers 409 and sends nothing more, so the caller tries again later.
func associationBusy(what string, cause error) error {
	return packageBusy(fmt.Sprintf("AWS is changing %s, so nothing was changed. Try again once AWS is done, and give package "+
		"associations of one domain dependsOn on each other and on the domain", what), cause)
}

// keyStoreAccess returns the key store access an association configuration gives: whether it is enabled, and the
// role.
func keyStoreAccess(c *types.PackageAssociationConfiguration) (bool, string) {
	if c == nil || c.KeyStoreAccessOption == nil {
		return false, ""
	}
	return aws.ToBool(c.KeyStoreAccessOption.KeyStoreAccessEnabled), aws.ToString(c.KeyStoreAccessOption.KeyAccessRoleArn)
}

// sameAssociation returns a rejected client failure when the domain already has the package with other prerequisites
// or another configuration than the template's, so a create never takes another association as the template's. AWS
// may not return either, so only values AWS returns are compared.
func sameAssociation(id associationID, want *opensearch.AssociatePackageInput, got *types.DomainPackageDetails) error {
	same := true
	if got.PrerequisitePackageIDList != nil {
		same = slices.Equal(slices.Sorted(slices.Values(want.PrerequisitePackageIDList)), slices.Sorted(slices.Values(got.PrerequisitePackageIDList)))
	}
	if got.AssociationConfiguration != nil && got.AssociationConfiguration.KeyStoreAccessOption != nil {
		wantEnabled, wantRole := keyStoreAccess(want.AssociationConfiguration)
		gotEnabled, gotRole := keyStoreAccess(got.AssociationConfiguration)
		same = same && wantEnabled == gotEnabled && (gotRole == "" || wantRole == gotRole)
	}
	if same {
		return nil
	}
	return &native.Failure{
		Message: fmt.Sprintf("%s: the domain %q already has the package %s with other PrerequisitePackageIDList or AssociationConfiguration values than the template's. "+
			"Set them to match, or dissociate the package first", packageAssociationType, id.domain, id.packageID),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Create associates the package with the domain. It reads the domain's packages first and sends nothing when the
// domain already has the package with the template's values, such as after a create whose response was lost, because
// associating a package again applies its latest version, which for a plugin is a blue/green deployment. An
// association that failed is sent again.
func (h *PackageAssociation) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, ok, err := parseAssociation(request.Properties)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", invalid(packageAssociationType, "set DomainName and PackageID")
	}
	in, err := associationOptions(request.Properties)
	if err != nil {
		return "", err
	}
	switch d, err := h.find(ctx, request.Region, id); {
	case err == nil:
		switch d.DomainPackageStatus {
		case types.DomainPackageStatusActive, types.DomainPackageStatusAssociating:
			if err := sameAssociation(id, in, d); err != nil {
				return "", err
			}
			return id.String(), nil
		case types.DomainPackageStatusDissociating:
			return "", associationBusy(fmt.Sprintf("the package %s of the domain %q (%s)", id.packageID, id.domain, d.DomainPackageStatus), nil)
		}
	case !errors.Is(err, native.ErrNotFound):
		return "", beforeChange(err)
	}
	in.DomainName, in.PackageID = aws.String(id.domain), aws.String(id.packageID)
	if _, err := h.api.AssociatePackage(ctx, in, inRegion(request.Region), sendOnce); err != nil {
		if awsapi.HasCode(err, "ConflictException") {
			return "", associationBusy(fmt.Sprintf("the domain %q or another of its packages", id.domain), err)
		}
		return "", awsapi.Rejected(err)
	}
	return id.String(), nil
}

// Update changes nothing, because every property is create-only.
func (h *PackageAssociation) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete dissociates the package from the domain. An association AWS is already dissociating had a delete reach AWS
// before, so the delete succeeds. One AWS is still associating gets a conflict, and nothing is sent. It returns
// ErrNotFound when the domain doesn't list the package.
func (h *PackageAssociation) Delete(ctx context.Context, region string, identifier string) error {
	id, err := parseAssociationID(identifier)
	if err != nil {
		return err
	}
	d, err := h.find(ctx, region, id)
	if err != nil {
		return beforeChange(err)
	}
	switch d.DomainPackageStatus {
	case types.DomainPackageStatusDissociating:
		return nil
	case types.DomainPackageStatusAssociating:
		return associationBusy(fmt.Sprintf("the package %s of the domain %q (%s)", id.packageID, id.domain, d.DomainPackageStatus), nil)
	}
	_, err = h.api.DissociatePackage(ctx, &opensearch.DissociatePackageInput{DomainName: aws.String(id.domain), PackageID: aws.String(id.packageID)}, inRegion(region))
	if awsapi.HasCode(err, "ConflictException") {
		return associationBusy(fmt.Sprintf("the domain %q or another of its packages, or an index uses the package", id.domain), err)
	}
	return awsapi.Rejected(packageNotFound(err))
}

// List returns nothing, because AWS lists associations only by domain or by package.
func (h *PackageAssociation) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the domain's packages. A create waits for ACTIVE and fails at ASSOCIATION_FAILED as rejected: the
// domain doesn't have the package, so the next deploy sends the create again, which associates it again. A delete
// waits until the domain no longer lists the package, when Progress returns ErrNotFound, and fails at
// DISSOCIATION_FAILED. An update changes nothing.
func (h *PackageAssociation) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	id, err := parseAssociationID(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	d, err := h.find(ctx, region, id)
	if err != nil {
		return native.Progress{}, err
	}
	status := string(d.DomainPackageStatus)
	details := ""
	if e := d.ErrorDetails; e != nil {
		details = fmt.Sprintf(": %s: %s", aws.ToString(e.ErrorType), aws.ToString(e.ErrorMessage))
	}
	if op == native.OperationCreate {
		switch d.DomainPackageStatus {
		case types.DomainPackageStatusActive:
			return native.Progress{Done: true, State: status}, nil
		case types.DomainPackageStatusAssociationFailed:
			// The domain doesn't have the package, so AWS changed nothing, and the next deploy associates it again.
			return native.Progress{Done: true, State: status, Failure: &native.Failure{Message: fmt.Sprintf(
				"AWS failed to associate the package %s with the domain %q%s. Check that the package is AVAILABLE and fits the domain's engine version, then deploy again",
				id.packageID, id.domain, details), Outcome: native.OutcomeRejected}}, nil
		}
		return native.Progress{State: status, Waiting: string(types.DomainPackageStatusActive)}, nil
	}
	if d.DomainPackageStatus == types.DomainPackageStatusDissociationFailed {
		return native.Progress{Done: true, State: status, Failure: &native.Failure{Message: fmt.Sprintf(
			"AWS failed to dissociate the package %s from the domain %q%s. Remove the package from every index that uses it, then delete again",
			id.packageID, id.domain, details)}}, nil
	}
	return native.Progress{State: status, Waiting: "dissociated"}, nil
}
