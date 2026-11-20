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
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	pkgAPIReference = "https://docs.aws.amazon.com/opensearch-service/latest/APIReference/"
	packagesGuide   = "https://docs.aws.amazon.com/opensearch-service/latest/developerguide/custom-packages.html"
	pluginsGuide    = "https://docs.aws.amazon.com/opensearch-service/latest/developerguide/custom-plugins.html"

	// pkgConstraintNote is what every package call's page leaves unclear about its constraints.
	pkgConstraintNote = " The page names no error for a value outside its length, pattern, or valid values, or for a required " +
		"value left out; the fake returns ValidationException, the 400 error the page lists."
)

// pkgCall is a call against a fake package API, after any calls it needs to set up.
type pkgCall func(ctx context.Context, f *pkgFake) error

// pkgAgainst returns calls as a rule's Break or Check, made in order against one fake API backed by store. It stops
// at the first call that fails, and returns its error.
func pkgAgainst(calls ...pkgCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newPkgFake(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// createPkgInput returns the input of a create of a dictionary package named dictionary from synonyms.txt, with the
// changes edits make.
func createPkgInput(edits ...func(*opensearch.CreatePackageInput)) *opensearch.CreatePackageInput {
	in := &opensearch.CreatePackageInput{
		PackageName:   aws.String("dictionary"),
		PackageType:   types.PackageTypeTxtDictionary,
		PackageSource: &types.PackageSource{S3BucketName: aws.String(pkgBucket), S3Key: aws.String("synonyms.txt")},
	}
	for _, edit := range edits {
		edit(in)
	}
	return in
}

func createPkg(edits ...func(*opensearch.CreatePackageInput)) pkgCall {
	return func(ctx context.Context, f *pkgFake) error {
		_, err := f.CreatePackage(ctx, createPkgInput(edits...))
		return err
	}
}

// asPlugin returns an edit that makes the create a plugin for the engine version, or the default when it is "".
func asPlugin(engine string) func(*opensearch.CreatePackageInput) {
	return func(in *opensearch.CreatePackageInput) {
		in.PackageName, in.PackageType = aws.String("plugin"), types.PackageTypeZipPlugin
		in.PackageSource.S3Key = aws.String("plugin.zip")
		if engine != "" {
			in.EngineVersion = aws.String(engine)
		}
	}
}

// describeAll returns every package DescribePackages returns for the filters, from every page.
func describeAll(ctx context.Context, f *pkgFake, filters ...types.DescribePackagesFilter) ([]types.PackageDetails, error) {
	out := []types.PackageDetails{}
	in := &opensearch.DescribePackagesInput{Filters: filters}
	for {
		page, err := f.DescribePackages(ctx, in)
		if err != nil {
			return nil, err
		}
		out = append(out, page.PackageDetailsList...)
		if in.NextToken = page.NextToken; in.NextToken == nil {
			return out, nil
		}
	}
}

// describeOne returns the package with the ID, or nil when AWS doesn't describe it.
func describeOne(ctx context.Context, f *pkgFake, id string) (*types.PackageDetails, error) {
	found, err := describeAll(ctx, f, types.DescribePackagesFilter{Name: types.DescribePackagesFilterNamePackageID, Value: []string{id}})
	if err != nil || len(found) == 0 {
		return nil, err
	}
	return &found[0], nil
}

// settlePackages reads every package until none is changing.
func settlePackages(ctx context.Context, f *pkgFake) error {
	for range 4*pkgSettleReads + 2 {
		if _, err := describeAll(ctx, f); err != nil {
			return err
		}
	}
	return nil
}

// statuses returns the statuses a package reports, read after read, until it ends.
func statuses(ctx context.Context, f *pkgFake, id string) ([]types.PackageStatus, error) {
	out := []types.PackageStatus{}
	for range 4*pkgSettleReads + 2 {
		p, err := describeOne(ctx, f, id)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return append(out, ""), nil
		}
		if len(out) == 0 || out[len(out)-1] != p.PackageStatus {
			out = append(out, p.PackageStatus)
		}
	}
	return out, nil
}

// associate associates the package with the domain logs, with the changes edits make.
func associate(packageID string, edits ...func(*opensearch.AssociatePackageInput)) pkgCall {
	return func(ctx context.Context, f *pkgFake) error {
		in := &opensearch.AssociatePackageInput{DomainName: aws.String("logs"), PackageID: aws.String(packageID)}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.AssociatePackage(ctx, in)
		return err
	}
}

// settleAssociations reads the domain's packages until none is changing.
func settleAssociations(domain string) pkgCall {
	return func(ctx context.Context, f *pkgFake) error {
		for range pkgSettleReads + 1 {
			if _, err := listAll(ctx, f, domain); err != nil {
				return err
			}
		}
		return nil
	}
}

// listAll returns every package of the domain, from every page of ListPackagesForDomain.
func listAll(ctx context.Context, f *pkgFake, domain string) ([]types.DomainPackageDetails, error) {
	out := []types.DomainPackageDetails{}
	in := &opensearch.ListPackagesForDomainInput{DomainName: aws.String(domain)}
	for {
		page, err := f.ListPackagesForDomain(ctx, in)
		if err != nil {
			return nil, err
		}
		out = append(out, page.DomainPackageDetailsList...)
		if in.NextToken = page.NextToken; in.NextToken == nil {
			return out, nil
		}
	}
}

// packageAPI is the record of the AWS calls the package handler makes.
var packageAPI = map[string]nativetest.Call{
	"CreatePackage": {
		Doc: pkgAPIReference + "API_CreatePackage.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "PackageName is 3 to 256 characters matching [a-z][a-z0-9\\-]+",
				Break: pkgAgainst(createPkg(func(in *opensearch.CreatePackageInput) { in.PackageName = aws.String("Dictionary") })),
				Code:  pkgValidationCode,
			},
			{
				Rule:  "PackageType is TXT-DICTIONARY, ZIP-PLUGIN, PACKAGE-LICENSE, or PACKAGE-CONFIG",
				Break: pkgAgainst(createPkg(func(in *opensearch.CreatePackageInput) { in.PackageType = "JAR" })),
				Code:  pkgValidationCode,
			},
			{
				Rule:  "PackageSource is required",
				Break: pkgAgainst(createPkg(func(in *opensearch.CreatePackageInput) { in.PackageSource = nil })),
				Code:  pkgValidationCode,
			},
			{
				Rule:  "S3BucketName is 3 to 63 characters",
				Doc:   pkgAPIReference + "API_PackageSource.html",
				Break: pkgAgainst(createPkg(func(in *opensearch.CreatePackageInput) { in.PackageSource.S3BucketName = aws.String("b") })),
				Code:  pkgValidationCode,
			},
			{
				Rule:  "EngineVersion can only be set for a ZIP-PLUGIN package",
				Break: pkgAgainst(createPkg(func(in *opensearch.CreatePackageInput) { in.EngineVersion = aws.String("OpenSearch_2.15") })),
				Code:  pkgValidationCode,
			},
			{
				Rule: "EngineVersion matches Elasticsearch_X.Y or OpenSearch_X.Y",
				Break: pkgAgainst(createPkg(asPlugin(""), func(in *opensearch.CreatePackageInput) {
					in.EngineVersion = aws.String("OpenSearch-2.15")
				})),
				Code: pkgValidationCode,
			},
			{
				Rule:  "PackageDescription is at most 1024 characters",
				Break: pkgAgainst(createPkg(func(in *opensearch.CreatePackageInput) { in.PackageDescription = aws.String(strings.Repeat("d", 1025)) })),
				Code:  pkgValidationCode,
			},
			{
				Rule: "PackageConfiguration requires ConfigurationRequirement and LicenseRequirement",
				Doc:  pkgAPIReference + "API_PackageConfiguration.html",
				Break: pkgAgainst(createPkg(asPlugin(""), func(in *opensearch.CreatePackageInput) {
					in.PackageConfiguration = &types.PackageConfiguration{ConfigurationRequirement: types.RequirementLevelNone}
				})),
				Code: pkgValidationCode,
			},
			{
				Rule: "LicenseFilepath is a relative path without '..'",
				Doc:  pkgAPIReference + "API_PackageConfiguration.html",
				Break: pkgAgainst(createPkg(asPlugin(""), func(in *opensearch.CreatePackageInput) {
					in.PackageConfiguration = &types.PackageConfiguration{
						ConfigurationRequirement: types.RequirementLevelNone, LicenseRequirement: types.RequirementLevelRequired,
						LicenseFilepath: aws.String("licenses/../secret"),
					}
				})),
				Code: pkgValidationCode,
			},
			{
				Rule: "PackageEncryptionOptions requires EncryptionEnabled",
				Doc:  pkgAPIReference + "API_PackageEncryptionOptions.html",
				Break: pkgAgainst(createPkg(func(in *opensearch.CreatePackageInput) {
					in.PackageEncryptionOptions = &types.PackageEncryptionOptions{KmsKeyIdentifier: aws.String("alias/packages")}
				})),
				Code: pkgValidationCode,
			},
			{
				Rule: "KmsKeyIdentifier is 1 to 500 characters",
				Doc:  pkgAPIReference + "API_PackageEncryptionOptions.html",
				Break: pkgAgainst(createPkg(func(in *opensearch.CreatePackageInput) {
					in.PackageEncryptionOptions = &types.PackageEncryptionOptions{EncryptionEnabled: aws.Bool(true), KmsKeyIdentifier: aws.String("")}
				})),
				Code: pkgValidationCode,
			},
			{
				Rule: "PackageVendingOptions requires VendingEnabled",
				Doc:  pkgAPIReference + "API_PackageVendingOptions.html",
				Break: pkgAgainst(createPkg(func(in *opensearch.CreatePackageInput) {
					in.PackageVendingOptions = &types.PackageVendingOptions{}
				})),
				Code: pkgValidationCode,
			},
			{
				Rule:  "PackageName is unique, so a second package with the name fails with ResourceAlreadyExistsException",
				Break: pkgAgainst(createPkg(), createPkg(func(in *opensearch.CreatePackageInput) { in.PackageSource.S3Key = aws.String("stopwords.txt") })),
				Code:  "ResourceAlreadyExistsException",
			},
			{
				Rule:  "a plugin whose EngineVersion is left out gets the default one, so a second such plugin with the name fails with ResourceAlreadyExistsException",
				Doc:   pkgAPIReference + "API_PackageDetails.html",
				Break: pkgAgainst(createPkg(asPlugin("")), createPkg(asPlugin(""))),
				Code:  "ResourceAlreadyExistsException",
			},
			{
				Rule:  "a plugin keeps its package name across engine versions",
				Doc:   pluginsGuide,
				Check: pkgAgainst(createPkg(asPlugin("OpenSearch_2.15")), createPkg(asPlugin("OpenSearch_2.17"))),
			},
			{
				Rule: "a plugin's EngineVersion defaults to the latest version of OpenSearch",
				Doc:  pkgAPIReference + "API_PackageDetails.html",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					out, err := newPkgFake(store).CreatePackage(ctx, createPkgInput(asPlugin("")))
					if err != nil {
						return err
					}
					if aws.ToString(out.PackageDetails.EngineVersion) == "" {
						return fmt.Errorf("the plugin has no EngineVersion")
					}
					return nil
				},
			},
			{
				Rule: "AWS copies and validates a new package before it is AVAILABLE, and a package whose file it can't copy ends COPY_FAILED",
				Doc:  packagesGuide,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newPkgFake(store)
					for key, want := range map[string][]types.PackageStatus{
						"synonyms.txt": {types.PackageStatusCopying, types.PackageStatusValidating, types.PackageStatusAvailable},
						"missing.txt":  {types.PackageStatusCopying, types.PackageStatusCopyFailed},
					} {
						out, err := f.CreatePackage(ctx, createPkgInput(func(in *opensearch.CreatePackageInput) {
							in.PackageName, in.PackageSource.S3Key = aws.String("import-"+strings.TrimSuffix(key, ".txt")), aws.String(key)
						}))
						if err != nil {
							return err
						}
						got, err := statuses(ctx, f, aws.ToString(out.PackageDetails.PackageID))
						if err != nil {
							return err
						}
						if fmt.Sprint(got) != fmt.Sprint(want) {
							return fmt.Errorf("an import of %s went %v, not %v", key, got, want)
						}
					}
					return nil
				},
			},
		},
		Unclear: "CreatePackage takes no idempotency token. Whether AWS checks the S3 object during the call: the fake takes " +
			"any key and ends a package whose file it doesn't have COPY_FAILED. How long an import takes: the fake reports " +
			"COPYING and VALIDATING for two reads each. Whether a name a deleted package had can be used again while AWS is " +
			"deleting it: the fake rejects it until the package is gone. Which permissions AWS checks on the S3 object and on " +
			"a KMS key." + pkgConstraintNote,
	},
	"DescribePackages": {
		Doc: pkgAPIReference + "API_DescribePackages.html",
		Rules: []nativetest.Rule{
			{
				Rule: "a filter's Name is PackageID, PackageName, PackageStatus, PackageType, EngineVersion, or PackageOwner",
				Doc:  pkgAPIReference + "API_DescribePackagesFilter.html",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := describeAll(ctx, f, types.DescribePackagesFilter{Name: "Name", Value: []string{"x"}})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "a filter has at least one value, each matching ^[0-9a-zA-Z\\*\\.\\_\\\\\\/\\?-]+$",
				Doc:  pkgAPIReference + "API_DescribePackagesFilter.html",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := describeAll(ctx, f, types.DescribePackagesFilter{Name: types.DescribePackagesFilterNamePackageName, Value: []string{"a b"}})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "MaxResults is at most 100",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.DescribePackages(ctx, &opensearch.DescribePackagesInput{MaxResults: 101})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "the list has only the packages that match every filter, across pages",
				Check: pkgAgainst(createPkg(), createPkg(asPlugin("")), func(ctx context.Context, f *pkgFake) error {
					found, err := describeAll(ctx, f, types.DescribePackagesFilter{Name: types.DescribePackagesFilterNamePackageType, Value: []string{"TXT-DICTIONARY"}})
					if err != nil {
						return err
					}
					if len(found) != 3 {
						return fmt.Errorf("the filter matched %d dictionaries, not 3", len(found))
					}
					return nil
				}),
			},
		},
		Unclear: "Whether the list has packages AWS owns, such as optional plugins, and packages other accounts share: the fake " +
			"has an AWS plugin. Whether AWS reports a deleted package as DELETED, and for how long: the fake does for two " +
			"reads. Whether a filter value's * and ? are wildcards: the fake matches values exactly. The page size: the fake " +
			"returns two packages a page." + pkgConstraintNote,
	},
	"UpdatePackage": {
		Doc: pkgAPIReference + "API_UpdatePackage.html",
		Rules: []nativetest.Rule{
			{
				Rule: "PackageID matches ^([FG][0-9]+)$|^(pkg-[a-f0-9]+)$",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.UpdatePackage(ctx, &opensearch.UpdatePackageInput{PackageID: aws.String("package-1"), PackageSource: createPkgInput().PackageSource})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "PackageSource is required",
				Break: pkgAgainst(createPkg(), settlePackages, func(ctx context.Context, f *pkgFake) error {
					_, err := f.UpdatePackage(ctx, &opensearch.UpdatePackageInput{PackageID: aws.String("F000000001")})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "CommitMessage is at most 160 characters",
				Break: pkgAgainst(createPkg(), settlePackages, func(ctx context.Context, f *pkgFake) error {
					_, err := f.UpdatePackage(ctx, &opensearch.UpdatePackageInput{
						PackageID: aws.String("F000000001"), PackageSource: createPkgInput().PackageSource, CommitMessage: aws.String(strings.Repeat("m", 161)),
					})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "a package that doesn't exist fails with ResourceNotFoundException",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.UpdatePackage(ctx, &opensearch.UpdatePackageInput{PackageID: aws.String("F999999999"), PackageSource: createPkgInput().PackageSource})
					return err
				}),
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "an update imports the file again as a new package version, which domains get only when the package is associated again",
				Doc:  packagesGuide,
				Check: pkgAgainst(createPkg(), settlePackages, associate("F000000001"), settleAssociations("logs"), func(ctx context.Context, f *pkgFake) error {
					in := &opensearch.UpdatePackageInput{PackageID: aws.String("F000000001"), PackageSource: createPkgInput().PackageSource}
					if _, err := f.UpdatePackage(ctx, in); err != nil {
						return err
					}
					if err := settlePackages(ctx, f); err != nil {
						return err
					}
					p, err := describeOne(ctx, f, "F000000001")
					if err != nil {
						return err
					}
					list, err := listAll(ctx, f, "logs")
					if err != nil {
						return err
					}
					if aws.ToString(p.AvailablePackageVersion) != "v2" || aws.ToString(list[0].PackageVersion) != "v1" {
						return fmt.Errorf("the package is %s and the domain has %s", aws.ToString(p.AvailablePackageVersion), aws.ToString(list[0].PackageVersion))
					}
					return nil
				}),
			},
		},
		Unclear: "What an update that leaves out PackageDescription or PackageEncryptionOptions does: the fake keeps them. " +
			"What an empty PackageDescription does: the fake keeps it as an empty description, and the handler reads a missing one as empty too. Whether AWS takes an update of a " +
			"package that isn't AVAILABLE: the fake rejects it with BaseException. How soon an update shows: the fake reports " +
			"the package AVAILABLE with its old version for one more read." + pkgConstraintNote,
	},
	"DeletePackage": {
		Doc: pkgAPIReference + "API_DeletePackage.html",
		Rules: []nativetest.Rule{
			{
				Rule: "PackageID matches ^([FG][0-9]+)$|^(pkg-[a-f0-9]+)$",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.DeletePackage(ctx, &opensearch.DeletePackageInput{PackageID: aws.String("package-1")})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "a package that doesn't exist fails with ResourceNotFoundException",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.DeletePackage(ctx, &opensearch.DeletePackageInput{PackageID: aws.String("F999999999")})
					return err
				}),
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "a package a domain uses can't be deleted: ConflictException",
				Break: pkgAgainst(createPkg(), settlePackages, associate("F000000001"), settleAssociations("logs"), func(ctx context.Context, f *pkgFake) error {
					_, err := f.DeletePackage(ctx, &opensearch.DeletePackageInput{PackageID: aws.String("F000000001")})
					return err
				}),
				Code: "ConflictException",
			},
		},
		Unclear: "Whether AWS deletes a package it is still importing or deleting, and with what error: the fake rejects it " +
			"with BaseException. How long a delete takes: the fake reports DELETING for two reads, then DELETED for two more.",
	},
	"GetPackageVersionHistory": {
		Doc: pkgAPIReference + "API_GetPackageVersionHistory.html",
		Rules: []nativetest.Rule{
			{
				Rule: "PackageID matches ^([FG][0-9]+)$|^(pkg-[a-f0-9]+)$",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.GetPackageVersionHistory(ctx, &opensearch.GetPackageVersionHistoryInput{PackageID: aws.String("package-1")})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "MaxResults is at most 100",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.GetPackageVersionHistory(ctx, &opensearch.GetPackageVersionHistoryInput{PackageID: aws.String("F111111111"), MaxResults: 101})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "a package that doesn't exist fails with ResourceNotFoundException",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.GetPackageVersionHistory(ctx, &opensearch.GetPackageVersionHistoryInput{PackageID: aws.String("F999999999")})
					return err
				}),
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "each version is listed with the commit message of the update that made it, across pages",
				Check: pkgAgainst(createPkg(), settlePackages, func(ctx context.Context, f *pkgFake) error {
					for _, message := range []string{"first", "second"} {
						in := &opensearch.UpdatePackageInput{PackageID: aws.String("F000000001"), PackageSource: createPkgInput().PackageSource, CommitMessage: aws.String(message)}
						if _, err := f.UpdatePackage(ctx, in); err != nil {
							return err
						}
						if err := settlePackages(ctx, f); err != nil {
							return err
						}
					}
					messages := []string{}
					in := &opensearch.GetPackageVersionHistoryInput{PackageID: aws.String("F000000001")}
					for {
						page, err := f.GetPackageVersionHistory(ctx, in)
						if err != nil {
							return err
						}
						for _, v := range page.PackageVersionHistoryList {
							messages = append(messages, aws.ToString(v.CommitMessage))
						}
						if in.NextToken = page.NextToken; in.NextToken == nil {
							break
						}
					}
					if fmt.Sprint(messages) != fmt.Sprint([]string{"", "first", "second"}) {
						return fmt.Errorf("the history lists the commit messages %q", messages)
					}
					return nil
				}),
			},
		},
		Unclear: "When AWS lists a version: the fake lists it only once its import is AVAILABLE, so an update that is still " +
			"importing can't be told from one made outside Radius. Whether a version whose import failed is listed: the fake " +
			"lists none. Whether PackageVersion has the form of AvailablePackageVersion: the handler compares only CreatedAt and " +
			"CommitMessage. The page size: the fake returns two versions a page." + pkgConstraintNote,
	},
	"GetCallerIdentity": {
		Doc: "https://docs.aws.amazon.com/STS/latest/APIReference/API_GetCallerIdentity.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the call needs no permissions and returns the account that owns the caller",
				Check: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					out, err := f.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
					if err != nil {
						return err
					}
					if aws.ToString(out.Account) != pkgAccount {
						return fmt.Errorf("the call returned the account %q", aws.ToString(out.Account))
					}
					return nil
				}),
			},
		},
		Unclear: "The OpenSearch docs don't say what PackageOwner holds: the fake reports the account ID for the account's " +
			"packages and AWS for AWS's, and the handler takes only packages whose owner is the account ID.",
	},
}

// associationAPI is the record of the AWS calls the package association handler makes.
var associationAPI = map[string]nativetest.Call{
	"AssociatePackage": {
		Doc: pkgAPIReference + "API_AssociatePackage.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "DomainName is 3 to 28 characters matching [a-z][a-z0-9\\-]+",
				Break: pkgAgainst(associate("F111111111", func(in *opensearch.AssociatePackageInput) { in.DomainName = aws.String("Logs") })),
				Code:  pkgValidationCode,
			},
			{
				Rule:  "PackageID and each prerequisite match ^([FG][0-9]+)$|^(pkg-[a-f0-9]+)$",
				Break: pkgAgainst(associate("F111111111", func(in *opensearch.AssociatePackageInput) { in.PrerequisitePackageIDList = []string{"plugin"} })),
				Code:  pkgValidationCode,
			},
			{
				Rule: "KeyStoreAccessOption requires KeyStoreAccessEnabled, and KeyAccessRoleArn is an IAM role ARN",
				Doc:  pkgAPIReference + "API_KeyStoreAccessOption.html",
				Break: pkgAgainst(associate("F111111111", func(in *opensearch.AssociatePackageInput) {
					in.AssociationConfiguration = &types.PackageAssociationConfiguration{KeyStoreAccessOption: &types.KeyStoreAccessOption{
						KeyStoreAccessEnabled: aws.Bool(true), KeyAccessRoleArn: aws.String("arn:aws:iam::123456789012:user/keys"),
					}}
				})),
				Code: pkgValidationCode,
			},
			{
				Rule:  "a domain that doesn't exist fails with ResourceNotFoundException",
				Break: pkgAgainst(associate("F111111111", func(in *opensearch.AssociatePackageInput) { in.DomainName = aws.String("missing") })),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule:  "a package that doesn't exist fails with ResourceNotFoundException",
				Break: pkgAgainst(associate("F999999999")),
				Code:  "ResourceNotFoundException",
			},
			{
				Rule:  "a plugin can only be associated with a domain of its engine version",
				Doc:   packagesGuide,
				Break: pkgAgainst(associate("pkg-0a1b2c3d", func(in *opensearch.AssociatePackageInput) { in.DomainName = aws.String("metrics") })),
				Code:  pkgBaseCode,
			},
			{
				Rule:  "the prerequisites must be associated with the domain first",
				Break: pkgAgainst(associate("pkg-1c2d3e4f", func(in *opensearch.AssociatePackageInput) { in.PrerequisitePackageIDList = []string{"pkg-0a1b2c3d"} })),
				Code:  pkgBaseCode,
			},
			{
				Rule: "a domain can have at most 20 plugins",
				Doc:  packagesGuide,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newPkgFake(store)
					for i := range pkgMaxPlugins {
						store.Put(packageAssociationType, fmt.Sprintf("logs|pkg-%x", 0xf00+i), map[string]any{"PackageType": "ZIP-PLUGIN"})
					}
					return associate("pkg-0a1b2c3d")(ctx, f)
				},
				Code: pkgBaseCode,
			},
			{
				Rule: "an association reports ASSOCIATING until the domain has the package, then ACTIVE",
				Doc:  pluginsGuide,
				Check: pkgAgainst(associate("F111111111"), func(ctx context.Context, f *pkgFake) error {
					first, err := listAll(ctx, f, "logs")
					if err != nil {
						return err
					}
					if err := settleAssociations("logs")(ctx, f); err != nil {
						return err
					}
					then, err := listAll(ctx, f, "logs")
					if err != nil {
						return err
					}
					if first[0].DomainPackageStatus != types.DomainPackageStatusAssociating || then[0].DomainPackageStatus != types.DomainPackageStatusActive {
						return fmt.Errorf("the association went %s, then %s", first[0].DomainPackageStatus, then[0].DomainPackageStatus)
					}
					return nil
				}),
			},
		},
		Unclear: "Whether AWS takes an association while it is changing the domain or another of its packages: the fake " +
			"rejects it with ConflictException, the conflict this page lists. What associating a package the domain already " +
			"has does: the docs use it to apply a new package version, so the fake associates it again. Whether a package must " +
			"be AVAILABLE: the fake rejects one that isn't with BaseException, as it does for the plugin limit, the engine " +
			"version, and the prerequisites, for which the page names no error. Whether AWS needs iam:PassRole for " +
			"KeyAccessRoleArn." + pkgConstraintNote,
	},
	"DissociatePackage": {
		Doc: pkgAPIReference + "API_DissociatePackage.html",
		Rules: []nativetest.Rule{
			{
				Rule: "DomainName is 3 to 28 characters matching [a-z][a-z0-9\\-]+",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.DissociatePackage(ctx, &opensearch.DissociatePackageInput{DomainName: aws.String("lo"), PackageID: aws.String("F111111111")})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "a domain that doesn't exist fails with ResourceNotFoundException",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.DissociatePackage(ctx, &opensearch.DissociatePackageInput{DomainName: aws.String("missing"), PackageID: aws.String("F111111111")})
					return err
				}),
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "dissociating a plugin also removes its configuration and license packages",
				Doc:  pluginsGuide,
				Check: pkgAgainst(associate("pkg-0a1b2c3d"), settleAssociations("logs"),
					associate("pkg-1c2d3e4f", func(in *opensearch.AssociatePackageInput) { in.PrerequisitePackageIDList = []string{"pkg-0a1b2c3d"} }),
					settleAssociations("logs"),
					func(ctx context.Context, f *pkgFake) error {
						_, err := f.DissociatePackage(ctx, &opensearch.DissociatePackageInput{DomainName: aws.String("logs"), PackageID: aws.String("pkg-0a1b2c3d")})
						return err
					},
					settleAssociations("logs"),
					func(ctx context.Context, f *pkgFake) error {
						if list, err := listAll(ctx, f, "logs"); err != nil || len(list) != 0 {
							return fmt.Errorf("the domain still has %d packages: %v", len(list), err)
						}
						return nil
					}),
			},
		},
		Unclear: "What dissociating a package the domain doesn't have does: the fake fails with ResourceNotFoundException. " +
			"Whether AWS takes a dissociation while it is changing the domain or another of its packages: the fake rejects " +
			"it with ConflictException. The developer guide says a dissociation fails while an index uses the package, but " +
			"not how: the fake can end it DISSOCIATION_FAILED." + pkgConstraintNote,
	},
	"ListPackagesForDomain": {
		Doc: pkgAPIReference + "API_ListPackagesForDomain.html",
		Rules: []nativetest.Rule{
			{
				Rule: "a domain that doesn't exist fails with ResourceNotFoundException",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := listAll(ctx, f, "missing")
					return err
				}),
				Code: "ResourceNotFoundException",
			},
			{
				Rule: "MaxResults is at most 100",
				Break: pkgAgainst(func(ctx context.Context, f *pkgFake) error {
					_, err := f.ListPackagesForDomain(ctx, &opensearch.ListPackagesForDomainInput{DomainName: aws.String("logs"), MaxResults: 101})
					return err
				}),
				Code: pkgValidationCode,
			},
			{
				Rule: "the list has every package of the domain, across pages",
				Check: pkgAgainst(associate("F111111111"), settleAssociations("logs"), associate("F222222222"), settleAssociations("logs"),
					associate("pkg-0a1b2c3d"), settleAssociations("logs"), func(ctx context.Context, f *pkgFake) error {
						list, err := listAll(ctx, f, "logs")
						if err != nil {
							return err
						}
						if len(list) != 3 {
							return fmt.Errorf("the list has %d packages, not 3", len(list))
						}
						return nil
					}),
			},
		},
		Unclear: "The page size: the fake returns two packages a page.",
	},
}
