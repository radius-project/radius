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
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/google/uuid"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const packageType = "AWS::OpenSearchService::Package"

// packageTimeout is how long UCP waits for each operation on a package. AWS doesn't say how long it takes to copy and
// validate one, which for a plugin includes a security scan.
const packageTimeout = time.Hour

var (
	_ native.Handler           = (*Package)(nil)
	_ native.Finder            = (*Package)(nil)
	_ native.UpdatePreparer    = (*Package)(nil)
	_ native.IdentifierEncoder = (*Package)(nil)
)

var (
	// packageNamePattern, packageIDPattern, and engineVersionPattern are the documented patterns of PackageName,
	// PackageID, and EngineVersion.
	packageNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9\-]+$`)
	packageIDPattern     = regexp.MustCompile(`^([FG][0-9]+)$|^(pkg-[a-f0-9]+)$`)
	engineVersionPattern = regexp.MustCompile(`^Elasticsearch_[0-9]{1}\.[0-9]{1,2}$|^OpenSearch_[0-9]{1,2}\.[0-9]{1,2}$`)

	// licenseFilepathPattern is the documented pattern of LicenseFilepath without its look-aheads, which reject "..";
	// checkPackageConfiguration checks those separately.
	licenseFilepathPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*$`)
)

var packageCreateStates = awsapi.States{
	Done:   []string{string(types.PackageStatusAvailable)},
	Failed: []string{string(types.PackageStatusCopyFailed), string(types.PackageStatusValidationFailed)},
}

// PackageAPI is the part of the Amazon OpenSearch Service client the package and package association handlers use.
type PackageAPI interface {
	CreatePackage(ctx context.Context, params *opensearch.CreatePackageInput, optFns ...func(*opensearch.Options)) (*opensearch.CreatePackageOutput, error)
	DescribePackages(ctx context.Context, params *opensearch.DescribePackagesInput, optFns ...func(*opensearch.Options)) (*opensearch.DescribePackagesOutput, error)
	UpdatePackage(ctx context.Context, params *opensearch.UpdatePackageInput, optFns ...func(*opensearch.Options)) (*opensearch.UpdatePackageOutput, error)
	DeletePackage(ctx context.Context, params *opensearch.DeletePackageInput, optFns ...func(*opensearch.Options)) (*opensearch.DeletePackageOutput, error)
	GetPackageVersionHistory(ctx context.Context, params *opensearch.GetPackageVersionHistoryInput, optFns ...func(*opensearch.Options)) (*opensearch.GetPackageVersionHistoryOutput, error)
	AssociatePackage(ctx context.Context, params *opensearch.AssociatePackageInput, optFns ...func(*opensearch.Options)) (*opensearch.AssociatePackageOutput, error)
	DissociatePackage(ctx context.Context, params *opensearch.DissociatePackageInput, optFns ...func(*opensearch.Options)) (*opensearch.DissociatePackageOutput, error)
	ListPackagesForDomain(ctx context.Context, params *opensearch.ListPackagesForDomainInput, optFns ...func(*opensearch.Options)) (*opensearch.ListPackagesForDomainOutput, error)
}

// PackageAccountAPI is the part of the STS client the package handler uses, to learn the current account.
type PackageAccountAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

var stsInRegion = awsapi.InRegion[sts.Options]

// sendOnce is the option of a call that changes a package or an association and takes no idempotency token, so the
// SDK never sends it again on its own. After a first attempt that reached AWS, a retried CreatePackage fails with
// ResourceAlreadyExistsException, which reads as a create AWS rejected, and a retried UpdatePackage or AssociatePackage
// imports the file or deploys the domain again. A connection error then reaches UCP, which finds what AWS has.
func sendOnce(o *opensearch.Options) {
	o.RetryMaxAttempts = 1
}

// Package serves AWS::OpenSearchService::Package: a dictionary, plugin, license, or configuration file AWS imports
// from S3 for domains to use. Its identifier, PackageID, is the ID AWS assigns. CreatePackage takes no idempotency
// token, but AWS keeps package names unique, so a lost create is found by its name (see Find). Every update imports
// PackageSource again, which makes a new package version.
type Package struct {
	api      PackageAPI
	accounts PackageAccountAPI
}

// NewPackage returns the handler for OpenSearch Service packages. It learns the current account with accounts, so a
// lost create is never found in a package another owner has.
func NewPackage(api PackageAPI, accounts PackageAccountAPI) *Package {
	return &Package{api: api, accounts: accounts}
}

// TypeName returns AWS::OpenSearchService::Package.
func (h *Package) TypeName() string {
	return packageType
}

// Options returns how UCP uses the handler. DescribePackages also returns AWS's own packages and packages other
// accounts share, and the docs don't say how PackageOwner tells them apart, so the type can't be listed.
func (h *Package) Options() native.Options {
	return native.Options{
		List:          native.ListNone,
		Recovery:      native.RecoverBySearch,
		CreateTimeout: packageTimeout,
		UpdateTimeout: packageTimeout,
		DeleteTimeout: packageTimeout,
	}
}

// errSource is the failure of a template without a valid PackageSource.
var errSource = invalid(packageType, "PackageSource must have S3BucketName, 3 to 63 characters, and S3Key, 1 to 1024 characters")

// Identifier returns PackageID when the properties give it, such as for `existing`, and "" otherwise, because AWS
// assigns it. UCP calls it before any AWS call, so a template with a value AWS would reject is rejected here. UCP also
// calls it with the properties of a lost create, which have no write-only values, so a template without
// PackageSource isn't rejected here: Create and Update require it.
func (h *Package) Identifier(properties map[string]any) (string, error) {
	if value, present := properties["PackageID"]; present && value != nil {
		id, isString := value.(string)
		if !isString {
			return "", invalid(packageType, "PackageID must be a string")
		}
		return id, checkPackageID(id)
	}
	_, err := packageInput(properties)
	return "", err
}

// sendable returns the create the template's properties describe, as packageInput does, and requires PackageSource,
// which every create and update sends.
func sendable(properties map[string]any) (*opensearch.CreatePackageInput, error) {
	in, err := packageInput(properties)
	if err == nil && in.PackageSource == nil {
		return nil, errSource
	}
	return in, err
}

// packageInput returns the create the template's properties describe, or a client failure for a value AWS would
// reject, so it is never sent. PackageSource may be left out.
func packageInput(properties map[string]any) (*opensearch.CreatePackageInput, error) {
	in := &opensearch.CreatePackageInput{}
	if err := awsapi.Decode(properties, in); err != nil {
		return nil, invalid(packageType, "a property doesn't have the schema's type: %v", err)
	}
	name := aws.ToString(in.PackageName)
	if len(name) < 3 || len(name) > 256 || !packageNamePattern.MatchString(name) {
		return nil, invalid(packageType, "PackageName must be 3 to 256 lowercase letters, digits, and hyphens, starting with a letter")
	}
	if !slices.Contains(in.PackageType.Values(), in.PackageType) {
		return nil, invalid(packageType, "PackageType must be one of %v", in.PackageType.Values())
	}
	if s := in.PackageSource; s != nil && (len(aws.ToString(s.S3BucketName)) < 3 || len(aws.ToString(s.S3BucketName)) > 63 ||
		aws.ToString(s.S3Key) == "" || len(aws.ToString(s.S3Key)) > 1024) {
		return nil, errSource
	}
	if in.EngineVersion != nil {
		if in.PackageType != types.PackageTypeZipPlugin {
			return nil, invalid(packageType, "EngineVersion is only for a ZIP-PLUGIN package")
		}
		if !engineVersionPattern.MatchString(*in.EngineVersion) {
			return nil, invalid(packageType, "EngineVersion must be OpenSearch_X.Y or Elasticsearch_X.Y")
		}
	}
	if len(aws.ToString(in.PackageDescription)) > 1024 {
		return nil, invalid(packageType, "PackageDescription must be at most 1024 characters")
	}
	if err := checkPackageConfiguration(in.PackageConfiguration); err != nil {
		return nil, err
	}
	if err := checkEncryptionOptions(in.PackageEncryptionOptions); err != nil {
		return nil, err
	}
	if o := in.PackageVendingOptions; o != nil && o.VendingEnabled == nil {
		return nil, invalid(packageType, "PackageVendingOptions must set VendingEnabled")
	}
	return in, nil
}

// checkPackageConfiguration returns a client failure for a configuration AWS would reject.
func checkPackageConfiguration(c *types.PackageConfiguration) error {
	if c == nil {
		return nil
	}
	levels := types.RequirementLevel("").Values()
	if !slices.Contains(levels, c.ConfigurationRequirement) || !slices.Contains(levels, c.LicenseRequirement) {
		return invalid(packageType, "PackageConfiguration must set ConfigurationRequirement and LicenseRequirement to one of %v", levels)
	}
	if path := c.LicenseFilepath; path != nil && (len(*path) > 256 || strings.Contains(*path, "..") || !licenseFilepathPattern.MatchString(*path)) {
		return invalid(packageType, "PackageConfiguration.LicenseFilepath must be a relative path of at most 256 characters, without '..'")
	}
	return nil
}

// checkEncryptionOptions returns a client failure for encryption options AWS would reject.
func checkEncryptionOptions(o *types.PackageEncryptionOptions) error {
	if o == nil {
		return nil
	}
	if o.EncryptionEnabled == nil {
		return invalid(packageType, "PackageEncryptionOptions must set EncryptionEnabled")
	}
	if key := o.KmsKeyIdentifier; key != nil && (*key == "" || len(*key) > 500) {
		return invalid(packageType, "PackageEncryptionOptions.KmsKeyIdentifier must be 1 to 500 characters")
	}
	return nil
}

// checkPackageID returns a client failure for an identifier that isn't a package ID, so it is never sent.
func checkPackageID(identifier string) error {
	if !packageIDPattern.MatchString(identifier) {
		return invalid(packageType, "%q is not the ID of an OpenSearch Service package, such as F123456789 or pkg-0123abcd", identifier)
	}
	return nil
}

// describePackages returns every package DescribePackages returns for the filter, from every page.
func (h *Package) describePackages(ctx context.Context, region string, filter types.DescribePackagesFilter) ([]types.PackageDetails, error) {
	out := []types.PackageDetails{}
	input := &opensearch.DescribePackagesInput{Filters: []types.DescribePackagesFilter{filter}}
	for {
		page, err := h.api.DescribePackages(ctx, input, inRegion(region))
		if err != nil {
			return nil, err
		}
		out = append(out, page.PackageDetailsList...)
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			return out, nil
		}
	}
}

// describe returns the package with the identifier, or ErrNotFound when AWS has none, or reports it deleted.
func (h *Package) describe(ctx context.Context, region string, identifier string) (*types.PackageDetails, error) {
	if err := checkPackageID(identifier); err != nil {
		return nil, err
	}
	found, err := h.describePackages(ctx, region, types.DescribePackagesFilter{
		Name:  types.DescribePackagesFilterNamePackageID,
		Value: []string{identifier},
	})
	if err != nil {
		return nil, awsapi.NotFound(err, "ResourceNotFoundException")
	}
	for i := range found {
		if aws.ToString(found[i].PackageID) != identifier {
			continue
		}
		if found[i].PackageStatus == types.PackageStatusDeleted {
			return nil, native.ErrNotFound
		}
		return &found[i], nil
	}
	return nil, native.ErrNotFound
}

// packageProperties returns the package's properties. PackageSource and PackageConfiguration are write-only, because
// AWS doesn't return them as sent.
func packageProperties(p *types.PackageDetails) (map[string]any, error) {
	properties, err := awsapi.EncodeObject(map[string]any{
		"PackageID":                p.PackageID,
		"PackageName":              p.PackageName,
		"PackageType":              string(p.PackageType),
		"EngineVersion":            p.EngineVersion,
		"PackageDescription":       p.PackageDescription,
		"PackageEncryptionOptions": p.PackageEncryptionOptions,
		"PackageVendingOptions":    p.PackageVendingOptions,
		"PackageOwner":             p.PackageOwner,
		"PackageStatus":            string(p.PackageStatus),
		"AvailablePackageVersion":  p.AvailablePackageVersion,
	})
	if err != nil {
		return nil, err
	}
	// A template can set an empty description, and AWS may return it empty or not at all, so the description is
	// always read, as "" when there is none. Otherwise every deploy of such a template would import the file again.
	properties["PackageDescription"] = aws.ToString(p.PackageDescription)
	return properties, nil
}

// Read returns the package's properties.
func (h *Package) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	p, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return packageProperties(p)
}

// Create starts importing the package.
func (h *Package) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	in, err := sendable(request.Properties)
	if err != nil {
		return "", err
	}
	out, err := h.api.CreatePackage(ctx, in, inRegion(request.Region), sendOnce)
	if err != nil {
		return "", awsapi.Rejected(err)
	}
	if out.PackageDetails == nil || aws.ToString(out.PackageDetails.PackageID) == "" {
		return "", fmt.Errorf("AWS created the OpenSearch Service package %q but returned no ID", aws.ToString(in.PackageName))
	}
	return aws.ToString(out.PackageDetails.PackageID), nil
}

// Find returns the package a lost create made: the one the current account owns with the template's name, type, and
// engine version, when the template sets one. AWS keeps names unique, except that the docs have a plugin keep its
// name across engine versions. DescribePackages also returns AWS's own packages and packages other accounts share, so
// Find leaves out every package whose PackageOwner isn't the account. A package whose other create-only values differ
// from the template's is another package, so Find fails rather than take it as the template's. request.Properties
// have no write-only values, so Find reads none.
func (h *Package) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	name := awsapi.String(request.Properties, "PackageName")
	if !packageNamePattern.MatchString(name) {
		return "", invalid(packageType, "PackageName must be 3 to 256 lowercase letters, digits, and hyphens, starting with a letter")
	}
	kind := types.PackageType(awsapi.String(request.Properties, "PackageType"))
	engine := awsapi.String(request.Properties, "EngineVersion")
	found, err := h.describePackages(ctx, request.Region, types.DescribePackagesFilter{
		Name:  types.DescribePackagesFilterNamePackageName,
		Value: []string{name},
	})
	if err != nil {
		return "", err
	}
	matches := []types.PackageDetails{}
	for _, p := range found {
		if aws.ToString(p.PackageName) != name || p.PackageType != kind || (engine != "" && aws.ToString(p.EngineVersion) != engine) ||
			p.PackageStatus == types.PackageStatusDeleting || p.PackageStatus == types.PackageStatusDeleted {
			continue
		}
		matches = append(matches, p)
	}
	if len(matches) > 0 {
		out, err := h.accounts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, stsInRegion(request.Region))
		if err != nil {
			return "", err
		}
		account := aws.ToString(out.Account)
		if account == "" {
			return "", fmt.Errorf("STS returned no account ID for the AWS credentials, so Radius can't tell which OpenSearch Service packages the account owns")
		}
		matches = slices.DeleteFunc(matches, func(p types.PackageDetails) bool { return aws.ToString(p.PackageOwner) != account })
	}
	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		// UCP keeps an identifier Find returns with an error, so a package that isn't the template's is never returned.
		if err := sameVending(request.Properties, &matches[0]); err != nil {
			return "", err
		}
		return aws.ToString(matches[0].PackageID), nil
	}
	ids := []string{}
	for _, p := range matches {
		ids = append(ids, aws.ToString(p.PackageID))
	}
	return "", awsapi.Ambiguous("OpenSearch Service packages", ids)
}

// sameVending returns a rejected client failure when the package's vending option, which is create-only, differs
// from the template's, so a lost create never takes another package as the template's.
func sameVending(properties map[string]any, p *types.PackageDetails) error {
	want := false
	if o, ok := properties["PackageVendingOptions"].(map[string]any); ok {
		want, _ = o["VendingEnabled"].(bool)
	}
	got := p.PackageVendingOptions != nil && aws.ToBool(p.PackageVendingOptions.VendingEnabled)
	if want == got {
		return nil
	}
	return &native.Failure{
		Message: fmt.Sprintf("%s: AWS has a package named %q (%s) whose VendingEnabled is %t, not the template's %t. Set PackageVendingOptions to match, or use another PackageName",
			packageType, aws.ToString(p.PackageName), aws.ToString(p.PackageID), got, want),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// packageBusy returns the conflict of a change AWS can't make now because it is importing or deleting the package,
// or because a domain uses it. UCP answers 409 and sends nothing more, so the caller tries again later.
func packageBusy(message string, cause error) error {
	if cause != nil {
		message += ": " + cause.Error()
	}
	return &native.Failure{Message: message, ClientError: true, Conflict: true, Outcome: native.OutcomeRejected}
}

// removesPath reports whether the update's patch removes path.
func removesPath(request native.UpdateRequest, path string) bool {
	return slices.ContainsFunc(request.Patch, func(op jsondiff.Operation) bool { return op.Type == jsondiff.OperationRemove && op.Path == path })
}

// Update imports PackageSource again with the template's description and encryption options, which makes a new
// package version. UpdatePackage requires PackageSource, so every update sends it. A removed description is sent as
// an empty one. AWS doesn't say how to remove encryption options, so an update that removes them is rejected. The
// update sends the commit message UpdateState chose for it, so an update sent again finds in the package's version
// history whether an earlier attempt reached AWS, and sends nothing then.
func (h *Package) Update(ctx context.Context, request native.UpdateRequest) error {
	in, err := sendable(request.Desired)
	if err != nil {
		return err
	}
	if removesPath(request, "/PackageEncryptionOptions") {
		return invalid(packageType, "AWS doesn't say how to remove a package's encryption options. Set PackageEncryptionOptions with EncryptionEnabled false instead")
	}
	var before packageVersion
	if request.State != "" {
		if before, err = savedVersion(request.Identifier, request.State); err != nil {
			return beforeChange(err)
		}
	}
	p, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return beforeChange(err)
	}
	if before.Commit != "" && before.changedTo(versionOf(p)) {
		// The package changed since UpdateState: by an earlier attempt of this update, or outside Radius.
		mine, _, err := h.committed(ctx, request.Region, request.Identifier, before.Commit)
		if err != nil {
			return beforeChange(err)
		}
		if mine != nil {
			return nil
		}
	}
	if err := updatable(request.Identifier, p); err != nil {
		return err
	}
	description := in.PackageDescription
	if description == nil && removesPath(request, "/PackageDescription") {
		description = aws.String("")
	}
	input := &opensearch.UpdatePackageInput{
		PackageID:                aws.String(request.Identifier),
		PackageSource:            in.PackageSource,
		PackageDescription:       description,
		PackageEncryptionOptions: in.PackageEncryptionOptions,
	}
	if before.Commit != "" {
		input.CommitMessage = aws.String(commitMessage(before.Commit))
	}
	_, err = h.api.UpdatePackage(ctx, input, inRegion(request.Region), sendOnce)
	return awsapi.Rejected(awsapi.NotFound(err, "ResourceNotFoundException"))
}

// updatable returns a conflict for a package AWS is still importing, and a rejected client failure for one it is
// deleting, so nothing is sent.
func updatable(identifier string, p *types.PackageDetails) error {
	switch p.PackageStatus {
	case types.PackageStatusCopying, types.PackageStatusValidating:
		return packageBusy(fmt.Sprintf("AWS is still importing the OpenSearch Service package %q (%s), so nothing was changed. Try again once it is AVAILABLE", identifier, p.PackageStatus), nil)
	case types.PackageStatusDeleting, types.PackageStatusDeleteFailed:
		return invalid(packageType, "AWS is deleting the package %q (%s), so it can't be updated", identifier, p.PackageStatus)
	}
	return nil
}

// commitMessage returns the commit message of the update that UpdateState chose commit for.
func commitMessage(commit string) string {
	return "Radius update " + commit
}

// committed returns the version of the package that the update with commit made, from the package's version history,
// or nil when AWS lists none, and whether it is the newest version AWS lists.
func (h *Package) committed(ctx context.Context, region string, identifier string, commit string) (*types.PackageVersionHistory, bool, error) {
	versions := []types.PackageVersionHistory{}
	input := &opensearch.GetPackageVersionHistoryInput{PackageID: aws.String(identifier)}
	for {
		page, err := h.api.GetPackageVersionHistory(ctx, input, inRegion(region))
		if err != nil {
			return nil, false, awsapi.NotFound(err, "ResourceNotFoundException")
		}
		versions = append(versions, page.PackageVersionHistoryList...)
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			break
		}
	}
	i := slices.IndexFunc(versions, func(v types.PackageVersionHistory) bool {
		return aws.ToString(v.CommitMessage) == commitMessage(commit)
	})
	if i < 0 {
		return nil, false, nil
	}
	mine := &versions[i]
	newest := !slices.ContainsFunc(versions, func(v types.PackageVersionHistory) bool {
		return aws.ToTime(v.CreatedAt).After(aws.ToTime(mine.CreatedAt))
	})
	return mine, newest, nil
}

// packageVersion is what UpdateState saves before an update: the package's version, status, and last update time,
// and Commit, a random value the update sends in its commit message, so the version it makes can be told apart from
// one made outside Radius.
type packageVersion struct {
	Version string    `json:"version,omitempty"`
	Status  string    `json:"status,omitempty"`
	Updated time.Time `json:"updated"`
	Commit  string    `json:"commit,omitempty"`
}

// changedTo reports whether the package's version, status, or update time moved on from v to now.
func (v packageVersion) changedTo(now packageVersion) bool {
	return now.Version != v.Version || now.Status != v.Status || !now.Updated.Equal(v.Updated)
}

// savedVersion returns the version UpdateState saved for the package.
func savedVersion(identifier string, state string) (packageVersion, error) {
	var v packageVersion
	if err := json.Unmarshal([]byte(state), &v); err != nil {
		return v, fmt.Errorf("failed to read the saved version of the OpenSearch Service package %q: %w", identifier, err)
	}
	return v, nil
}

func versionOf(p *types.PackageDetails) packageVersion {
	return packageVersion{Version: aws.ToString(p.AvailablePackageVersion), Status: string(p.PackageStatus), Updated: aws.ToTime(p.LastUpdatedAt)}
}

// UpdateState returns the package's version before the update, and a new commit value for it. AWS may report the
// package AVAILABLE for a while after UpdatePackage returns, before it starts importing the new version, so
// UpdateProgress waits for the version with the update's commit message. A package AWS is still importing or deleting
// gets the error Update would return, so the saved version is never one that an earlier change is about to replace.
func (h *Package) UpdateState(ctx context.Context, region string, identifier string) (string, error) {
	p, err := h.describe(ctx, region, identifier)
	if err != nil {
		return "", beforeChange(err)
	}
	if err := updatable(identifier, p); err != nil {
		return "", err
	}
	v := versionOf(p)
	v.Commit = uuid.NewString()
	b, err := json.Marshal(v)
	return string(b), err
}

// UpdateProgress reads the package's status. The update is done once AWS reports the package AVAILABLE, changed since
// state, with the update's own version as the newest in its history. It fails once AWS reports COPY_FAILED or
// VALIDATION_FAILED after state, and when another version replaced the update's, so UCP never records the template's
// file as what AWS has.
func (h *Package) UpdateProgress(ctx context.Context, region string, identifier string, state string) (native.Progress, error) {
	p, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	var before packageVersion
	if state != "" {
		if before, err = savedVersion(identifier, state); err != nil {
			return native.Progress{}, err
		}
	}
	now := versionOf(p)
	changed := now.Version != before.Version || !now.Updated.Equal(before.Updated)
	switch p.PackageStatus {
	case types.PackageStatusAvailable:
		if !changed {
			return native.Progress{State: "AVAILABLE with the old version", Waiting: "AVAILABLE with a new version"}, nil
		}
		mine, newest, err := h.committed(ctx, region, identifier, before.Commit)
		if err != nil {
			return native.Progress{}, err
		}
		if mine == nil {
			return native.Progress{State: "AVAILABLE with another version", Waiting: "AVAILABLE with the update's version"}, nil
		}
		if !newest {
			return native.Progress{Done: true, State: now.Status, Failure: &native.Failure{Message: fmt.Sprintf(
				"another update replaced the version of the OpenSearch Service package %q that Radius imported. Deploy again to import the template's file", identifier)}}, nil
		}
		return native.Progress{Done: true, State: now.Status}, nil
	case types.PackageStatusCopyFailed, types.PackageStatusValidationFailed:
		if changed || now.Status != before.Status {
			return packageFailed(p, "update"), nil
		}
	case types.PackageStatusDeleting, types.PackageStatusDeleteFailed:
		return native.Progress{Done: true, State: now.Status, Failure: &native.Failure{
			Message: fmt.Sprintf("AWS is deleting the OpenSearch Service package %q, so the update can't finish", identifier),
		}}, nil
	}
	return native.Progress{State: now.Status, Waiting: string(types.PackageStatusAvailable)}, nil
}

// packageFailed returns the failed progress of a create or update, with AWS's error details.
func packageFailed(p *types.PackageDetails, what string) native.Progress {
	message := fmt.Sprintf("AWS failed to %s the OpenSearch Service package %q (%s)", what, aws.ToString(p.PackageID), p.PackageStatus)
	if d := p.ErrorDetails; d != nil {
		message += fmt.Sprintf(": %s: %s", aws.ToString(d.ErrorType), aws.ToString(d.ErrorMessage))
	}
	if what == "create" {
		message += ". Fix the file PackageSource points to, then delete the resource, which removes the failed package, and deploy again"
	} else {
		message += ". Fix the file PackageSource points to and deploy again. If AWS rejects that update, delete the resource and deploy again"
	}
	return native.Progress{Done: true, State: string(p.PackageStatus), Failure: &native.Failure{Message: message}}
}

// Delete starts deleting the package. A package AWS is already deleting had a delete reach AWS before, so the delete
// succeeds. A package AWS is still importing, or that a domain uses, gets a conflict.
func (h *Package) Delete(ctx context.Context, region string, identifier string) error {
	p, err := h.describe(ctx, region, identifier)
	if err != nil {
		return beforeChange(err)
	}
	switch p.PackageStatus {
	case types.PackageStatusDeleting:
		return nil
	case types.PackageStatusCopying, types.PackageStatusValidating:
		return packageBusy(fmt.Sprintf("AWS is still importing the OpenSearch Service package %q (%s), so it wasn't deleted. Try again once it is AVAILABLE", identifier, p.PackageStatus), nil)
	}
	_, err = h.api.DeletePackage(ctx, &opensearch.DeletePackageInput{PackageID: aws.String(identifier)}, inRegion(region))
	if awsapi.HasCode(err, "ConflictException") {
		return packageBusy(fmt.Sprintf("AWS can't delete the OpenSearch Service package %q while a domain uses it, so it wasn't deleted. "+
			"Delete its package associations first, and give each AWS::OpenSearchService::PackageAssociation dependsOn on the package", identifier), err)
	}
	return awsapi.Rejected(awsapi.NotFound(err, "ResourceNotFoundException"))
}

// List returns nothing: the type can't be listed (see Options).
func (h *Package) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the package's status. A create waits for AVAILABLE and fails at COPY_FAILED or VALIDATION_FAILED,
// which leave the package in AWS, so the record stays and a later delete removes it. UCP waits for an update with
// UpdateProgress. A delete waits for the package to be gone, and fails at DELETE_FAILED.
func (h *Package) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	p, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	switch op {
	case native.OperationCreate, native.OperationUpdate:
		progress := packageCreateStates.Progress(string(p.PackageStatus))
		if progress.Failure != nil {
			return packageFailed(p, strings.ToLower(string(op))), nil
		}
		return progress, nil
	}
	if p.PackageStatus == types.PackageStatusDeleteFailed {
		return native.Progress{Done: true, State: string(p.PackageStatus), Failure: &native.Failure{
			Message: fmt.Sprintf("AWS failed to delete the OpenSearch Service package %q. Delete again to retry", identifier),
		}}, nil
	}
	return native.Progress{State: string(p.PackageStatus), Waiting: "deleted"}, nil
}
