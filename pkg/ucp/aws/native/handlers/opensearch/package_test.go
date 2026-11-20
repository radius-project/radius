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
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// readSchemaFile returns the hand-written schema or metadata file.
func readSchemaFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// withProps returns a copy of properties with changes.
func withProps(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	maps.Copy(out, changes)
	return out
}

// source returns a PackageSource for the key in the fake's bucket.
func source(key string) map[string]any {
	return map[string]any{"S3BucketName": pkgBucket, "S3Key": key}
}

// dictionaryTemplate is a template for a dictionary package imported from synonyms.txt.
func dictionaryTemplate() map[string]any {
	return map[string]any{
		"PackageName":        "dictionary",
		"PackageType":        "TXT-DICTIONARY",
		"PackageSource":      source("synonyms.txt"),
		"PackageDescription": "Synonyms",
	}
}

func Test_Suite_Package(t *testing.T) {
	properties := dictionaryTemplate()
	nativetest.Run(t, nativetest.Suite{
		TypeName:    packageType,
		Schema:      readSchemaFile(t, "aws-opensearchservice-package.json"),
		Metadata:    readSchemaFile(t, "aws-opensearchservice-package.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewPackage(newPkgFake(store))
		},
		IdentifierProperty: "PackageID",
		WriteOnly:          []string{"PackageSource", "PackageConfiguration"},
		Properties:         properties,
		Changed:            withProps(properties, map[string]any{"PackageDescription": "Synonyms, version 2"}),
		ChangedWriteOnly:   withProps(properties, map[string]any{"PackageSource": source("synonyms-2.txt")}),
		Removable:          "PackageDescription",
		CreateOnlyChange:   withProps(properties, map[string]any{"PackageName": "dictionary-2"}),
		Invalid: []map[string]any{
			withProps(properties, map[string]any{"PackageName": "Dictionary"}),
			withProps(properties, map[string]any{"PackageType": "JAR"}),
			withProps(properties, map[string]any{"EngineVersion": "OpenSearch_2.15"}),
			withProps(properties, map[string]any{"PackageSource": map[string]any{"S3BucketName": pkgBucket}}),
			withProps(properties, map[string]any{"PackageDescription": strings.Repeat("d", 1025)}),
			withProps(properties, map[string]any{"PackageEncryptionOptions": map[string]any{"KmsKeyIdentifier": "alias/packages"}}),
			withProps(properties, map[string]any{"PackageVendingOptions": map[string]any{}}),
			withProps(properties, map[string]any{"PackageName": 5}),
			withProps(properties, map[string]any{"PackageType": "ZIP-PLUGIN", "EngineVersion": "OpenSearch-2.15"}),
			withProps(properties, map[string]any{"PackageEncryptionOptions": map[string]any{"EncryptionEnabled": true, "KmsKeyIdentifier": ""}}),
			withProps(properties, map[string]any{"PackageConfiguration": map[string]any{"ConfigurationRequirement": "NONE"}}),
			withProps(properties, map[string]any{"PackageConfiguration": map[string]any{
				"ConfigurationRequirement": "NONE", "LicenseRequirement": "REQUIRED", "LicenseFilepath": "licenses/../key",
			}}),
		},
		API: packageAPI,
	})
}

func Test_Suite_PackageAssociation(t *testing.T) {
	properties := map[string]any{"DomainName": "logs", "PackageID": "F111111111"}
	nativetest.Run(t, nativetest.Suite{
		TypeName:    packageAssociationType,
		Schema:      readSchemaFile(t, "aws-opensearchservice-packageassociation.json"),
		Metadata:    readSchemaFile(t, "aws-opensearchservice-packageassociation.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewPackageAssociation(newPkgFake(store))
		},
		Properties: properties,
		Identifier: "logs|F111111111",
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          properties,
		CreateOnlyChange: withProps(properties, map[string]any{"PackageID": "F222222222"}),
		Invalid: []map[string]any{
			withProps(properties, map[string]any{"DomainName": "Logs"}),
			withProps(properties, map[string]any{"PackageID": "package-1"}),
			withProps(properties, map[string]any{"PrerequisitePackageIDList": []any{"plugin"}}),
			withProps(properties, map[string]any{"PrerequisitePackageIDList": "F222222222"}),
			withProps(properties, map[string]any{"AssociationConfiguration": map[string]any{"KeyStoreAccessOption": map[string]any{
				"KeyStoreAccessEnabled": true, "KeyAccessRoleArn": "arn:aws:iam::123456789012:user/keys",
			}}}),
			withProps(properties, map[string]any{"AssociationConfiguration": map[string]any{"KeyStoreAccessOption": map[string]any{}}}),
			withProps(properties, map[string]any{"AssociationConfiguration": "keys"}),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseAssociationID(identifier)
			require.NoError(t, err)
			return map[string]any{"DomainName": id.domain, "PackageID": id.packageID}
		},
		API: associationAPI,
	})
}

func newPackageHandler() (*Package, *pkgFake) {
	api := newPkgFake(awsfake.NewCloudControl())
	return NewPackage(api), api
}

// pkgWait polls the progress of op until it is done, and fails the test if it never is.
func pkgWait(t *testing.T, h native.Handler, identifier string, op native.Operation) native.Progress {
	t.Helper()
	for range 20 {
		p, err := h.Progress(t.Context(), pkgRegion, identifier, op)
		if op == native.OperationDelete && errors.Is(err, native.ErrNotFound) {
			return native.Progress{Done: true}
		}
		require.NoError(t, err)
		if p.Done {
			return p
		}
	}
	require.FailNow(t, "the operation never finished")
	return native.Progress{}
}

// createPackage creates a package with the handler and waits until it is AVAILABLE.
func createPackage(t *testing.T, h *Package, properties map[string]any) string {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: properties})
	require.NoError(t, err)
	require.Nil(t, pkgWait(t, h, id, native.OperationCreate).Failure)
	return id
}

// packageUpdate returns the update of the package from what AWS has to desired, with the patch UCP computes.
func packageUpdate(t *testing.T, h *Package, id string, desired map[string]any) native.UpdateRequest {
	t.Helper()
	current, err := h.Read(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	return native.UpdateRequest{Region: pkgRegion, Identifier: id, Current: current, Desired: desired, Patch: patch}
}

// updatePackage updates the package as UCP does, saving its state first, and waits for the update to end.
func updatePackage(t *testing.T, h *Package, request native.UpdateRequest) native.Progress {
	t.Helper()
	state, err := h.UpdateState(t.Context(), pkgRegion, request.Identifier)
	require.NoError(t, err)
	require.NoError(t, h.Update(t.Context(), request))
	for range 20 {
		p, err := h.UpdateProgress(t.Context(), pkgRegion, request.Identifier, state)
		require.NoError(t, err)
		if p.Done {
			return p
		}
	}
	require.FailNow(t, "the update never finished")
	return native.Progress{}
}

// requirePkgConflict checks that err is a conflict that changed nothing and names dependsOn or what to wait for.
func requirePkgConflict(t *testing.T, err error, mentions string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict, "%v", err)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, mentions)
}

// requirePkgRejected checks that err is a failure AWS applied none of: a rejected client failure, or a client fault
// from the fake, which UCP takes as rejected. The real SDK's 4xx errors become rejected failures (see the replay
// tests).
func requirePkgRejected(t *testing.T, err error) {
	t.Helper()
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		require.Equal(t, smithy.FaultClient, apiErr.ErrorFault(), "%v", err)
		return
	}
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError, "%v", err)
	require.False(t, failure.Conflict, "%v", err)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
}

// requirePkgRejectedBefore checks that err is a failure from before any change, so UCP drops the pending values, and
// that it names cause.
func requirePkgRejectedBefore(t *testing.T, err error, cause string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeRejected, failure.Outcome, "%v", err)
	require.Contains(t, failure.Message, cause)
}

// Test_Package_FirstLook checks that a new package isn't AVAILABLE the first time UCP reads it, and has a version once
// it is.
func Test_Package_FirstLook(t *testing.T) {
	h, _ := newPackageHandler()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: dictionaryTemplate()})
	require.NoError(t, err)
	p, err := h.Progress(t.Context(), pkgRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, p.Done)
	require.Equal(t, "AVAILABLE", p.Waiting)

	pkgWait(t, h, id, native.OperationCreate)
	properties, err := h.Read(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"PackageID": id, "PackageName": "dictionary", "PackageType": "TXT-DICTIONARY", "PackageDescription": "Synonyms",
		"PackageOwner": pkgAccount, "PackageStatus": "AVAILABLE", "AvailablePackageVersion": "v1",
	}, properties, "PackageSource is write-only, so Read leaves it out")
}

// Test_Package_ImportFails checks that a package AWS can't copy or validate fails the create with AWS's details, and
// stays in AWS for a delete to remove.
func Test_Package_ImportFails(t *testing.T) {
	h, api := newPackageHandler()
	for key, status := range map[string]string{"missing.txt": "COPY_FAILED", "bad-plugin.zip": "VALIDATION_FAILED"} {
		properties := withProps(dictionaryTemplate(), map[string]any{"PackageName": "p-" + strings.TrimSuffix(key, ".txt")[:3], "PackageSource": source(key)})
		id, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: properties})
		require.NoError(t, err)
		p := pkgWait(t, h, id, native.OperationCreate)
		require.NotNil(t, p.Failure, key)
		require.Equal(t, status, p.State)
		require.Contains(t, p.Failure.Message, "PackageSource")
		require.NotNil(t, api.store.Resource(packageType, id), "the package stays in AWS")

		require.NoError(t, h.Delete(t.Context(), pkgRegion, id))
		pkgWait(t, h, id, native.OperationDelete)
	}
}

// Test_Package_UpdateWaitsForNewVersion checks that an update isn't done while AWS still reports the old version as
// AVAILABLE, which it does for a while after UpdatePackage returns, and ends with a new version.
func Test_Package_UpdateWaitsForNewVersion(t *testing.T) {
	h, _ := newPackageHandler()
	id := createPackage(t, h, dictionaryTemplate())
	request := packageUpdate(t, h, id, withProps(dictionaryTemplate(), map[string]any{"PackageSource": source("synonyms-2.txt")}))
	state, err := h.UpdateState(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	require.NoError(t, h.Update(t.Context(), request))

	p, err := h.UpdateProgress(t.Context(), pkgRegion, id, state)
	require.NoError(t, err)
	require.False(t, p.Done, "AWS still reports the old version")
	require.Equal(t, "AVAILABLE with a new version", p.Waiting)

	for !p.Done {
		p, err = h.UpdateProgress(t.Context(), pkgRegion, id, state)
		require.NoError(t, err)
	}
	require.Nil(t, p.Failure)
	properties, err := h.Read(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	require.Equal(t, "v2", properties["AvailablePackageVersion"])

	_, err = h.UpdateProgress(t.Context(), pkgRegion, id, "{")
	require.ErrorContains(t, err, "saved version")
}

// Test_Package_UpdateFails checks that an update whose file AWS can't validate fails with AWS's details.
func Test_Package_UpdateFails(t *testing.T) {
	h, _ := newPackageHandler()
	id := createPackage(t, h, dictionaryTemplate())
	p := updatePackage(t, h, packageUpdate(t, h, id, withProps(dictionaryTemplate(), map[string]any{"PackageSource": source("bad-plugin.zip")})))
	require.NotNil(t, p.Failure)
	require.Equal(t, "VALIDATION_FAILED", p.State)
	require.Contains(t, p.Failure.Message, "could not be loaded")

	// AWS doesn't take an update of a package that isn't AVAILABLE.
	err := h.Update(t.Context(), packageUpdate(t, h, id, dictionaryTemplate()))
	requirePkgRejected(t, err)
}

// Test_Package_UpdateDescription checks that an update sends PackageSource, which UpdatePackage requires, and that
// removing the description sends an empty one, which clears it.
func Test_Package_UpdateDescription(t *testing.T) {
	h, api := newPackageHandler()
	id := createPackage(t, h, dictionaryTemplate())
	updates := &updateInputs{pkgFake: api}
	h = NewPackage(updates)

	require.Nil(t, updatePackage(t, h, packageUpdate(t, h, id, withProps(dictionaryTemplate(), map[string]any{"PackageDescription": "Synonyms 2"}))).Failure)
	require.Equal(t, "synonyms.txt", aws.ToString(updates.inputs[0].PackageSource.S3Key))
	require.Equal(t, "Synonyms 2", aws.ToString(updates.inputs[0].PackageDescription))

	without := dictionaryTemplate()
	delete(without, "PackageDescription")
	require.Nil(t, updatePackage(t, h, packageUpdate(t, h, id, without)).Failure)
	require.Equal(t, "", aws.ToString(updates.inputs[1].PackageDescription))
	require.NotNil(t, updates.inputs[1].PackageDescription)
	properties, err := h.Read(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	require.NotContains(t, properties, "PackageDescription")

	// A description AWS has that the template never set isn't removed.
	api.store.Resource(packageType, id)["PackageDescription"] = "outside"
	request := packageUpdate(t, h, id, withProps(without, map[string]any{"PackageSource": source("stopwords.txt")}))
	request.Patch = jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/PackageSource/S3Key", Value: "stopwords.txt"}}
	require.Nil(t, updatePackage(t, h, request).Failure)
	require.Nil(t, updates.inputs[2].PackageDescription)
}

// updateInputs records the input of each UpdatePackage.
type updateInputs struct {
	*pkgFake
	inputs []*opensearch.UpdatePackageInput
}

func (u *updateInputs) UpdatePackage(ctx context.Context, in *opensearch.UpdatePackageInput, opts ...func(*opensearch.Options)) (*opensearch.UpdatePackageOutput, error) {
	u.inputs = append(u.inputs, in)
	return u.pkgFake.UpdatePackage(ctx, in, opts...)
}

// Test_Package_UpdateEncryption checks that an update changes the encryption options, and that one that removes them
// is rejected before any AWS call, because AWS doesn't say how.
func Test_Package_UpdateEncryption(t *testing.T) {
	h, api := newPackageHandler()
	encrypted := withProps(dictionaryTemplate(), map[string]any{"PackageEncryptionOptions": map[string]any{"EncryptionEnabled": true, "KmsKeyIdentifier": "alias/a"}})
	id := createPackage(t, h, encrypted)
	changed := withProps(encrypted, map[string]any{"PackageEncryptionOptions": map[string]any{"EncryptionEnabled": true, "KmsKeyIdentifier": "alias/b"}})
	require.Nil(t, updatePackage(t, h, packageUpdate(t, h, id, changed)).Failure)
	properties, err := h.Read(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"EncryptionEnabled": true, "KmsKeyIdentifier": "alias/b"}, properties["PackageEncryptionOptions"])

	calls := api.calls["DescribePackages"]
	err = h.Update(t.Context(), packageUpdate(t, h, id, dictionaryTemplate()))
	requirePkgRejectedBefore(t, err, "EncryptionEnabled false")
	require.Equal(t, calls+1, api.calls["DescribePackages"], "only the read that built the request reached AWS")
	require.Equal(t, 1, api.calls["UpdatePackage"])
}

// Test_Package_Busy checks that an update or delete while AWS is still importing the package is a conflict that sends
// nothing, that a delete of a package a domain uses is a conflict that names dependsOn, and that an update of a
// package AWS is deleting is rejected.
func Test_Package_Busy(t *testing.T) {
	h, api := newPackageHandler()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: dictionaryTemplate()})
	require.NoError(t, err)
	requirePkgConflict(t, h.Update(t.Context(), native.UpdateRequest{Region: pkgRegion, Identifier: id, Desired: dictionaryTemplate()}), "still importing")
	requirePkgConflict(t, h.Delete(t.Context(), pkgRegion, id), "still importing")
	require.Zero(t, api.calls["UpdatePackage"]+api.calls["DeletePackage"])
	pkgWait(t, h, id, native.OperationCreate)

	associations := NewPackageAssociation(api)
	association := createAssociation(t, associations, map[string]any{"DomainName": "logs", "PackageID": id})
	requirePkgConflict(t, h.Delete(t.Context(), pkgRegion, id), "dependsOn")
	require.NoError(t, associations.Delete(t.Context(), pkgRegion, association))
	pkgWait(t, associations, association, native.OperationDelete)

	require.NoError(t, h.Delete(t.Context(), pkgRegion, id))
	requirePkgRejected(t, h.Update(t.Context(), native.UpdateRequest{Region: pkgRegion, Identifier: id, Desired: dictionaryTemplate()}))
	require.NoError(t, h.Delete(t.Context(), pkgRegion, id), "a delete AWS already started succeeds")
	require.Equal(t, 2, api.calls["DeletePackage"], "the delete AWS rejected while the domain used the package, and the one it took")
	require.Zero(t, api.calls["UpdatePackage"])

	pkgWait(t, h, id, native.OperationDelete)
	_, err = h.Read(t.Context(), pkgRegion, id)
	require.ErrorIs(t, err, native.ErrNotFound, "a package AWS reports DELETED is gone")
	require.ErrorIs(t, h.Delete(t.Context(), pkgRegion, id), native.ErrNotFound)
}

// Test_Package_Find checks how a lost create is found: by the template's name and type, and engine version when the
// template sets one, never taking a package with other create-only values, and failing when more than one matches.
func Test_Package_Find(t *testing.T) {
	h, api := newPackageHandler()
	find := func(properties map[string]any) (string, error) {
		return h.Find(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: properties})
	}
	withoutSource := func(properties map[string]any) map[string]any {
		out := maps.Clone(properties)
		delete(out, "PackageSource")
		return out
	}
	found, err := find(withoutSource(dictionaryTemplate()))
	require.NoError(t, err)
	require.Empty(t, found)

	id := createPackage(t, h, dictionaryTemplate())
	found, err = find(withoutSource(dictionaryTemplate()))
	require.NoError(t, err)
	require.Equal(t, id, found)

	// AWS's own plugin with the name isn't a dictionary.
	found, err = find(withProps(withoutSource(dictionaryTemplate()), map[string]any{"PackageName": "analysis-icu"}))
	require.NoError(t, err)
	require.Empty(t, found)

	_, err = find(withProps(withoutSource(dictionaryTemplate()), map[string]any{"PackageVendingOptions": map[string]any{"VendingEnabled": true}}))
	requirePkgRejectedBefore(t, err, "VendingEnabled")

	plugin := map[string]any{"PackageName": "plugin", "PackageType": "ZIP-PLUGIN", "PackageSource": source("plugin.zip")}
	first := createPackage(t, h, withProps(plugin, map[string]any{"EngineVersion": "OpenSearch_2.15"}))
	second := createPackage(t, h, withProps(plugin, map[string]any{"EngineVersion": "OpenSearch_2.17"}))
	found, err = find(withProps(withoutSource(plugin), map[string]any{"EngineVersion": "OpenSearch_2.17"}))
	require.NoError(t, err)
	require.Equal(t, second, found)
	_, err = find(withoutSource(plugin))
	require.ErrorContains(t, err, first)
	require.ErrorContains(t, err, second)

	require.NoError(t, h.Delete(t.Context(), pkgRegion, id))
	found, err = find(withoutSource(dictionaryTemplate()))
	require.NoError(t, err)
	require.Empty(t, found, "a package AWS is deleting isn't the lost create")

	_, err = find(map[string]any{"PackageName": "Dictionary"})
	requirePkgRejected(t, err)
	api.errs["DescribePackages"] = errors.New("describe down")
	_, err = find(withoutSource(dictionaryTemplate()))
	require.ErrorContains(t, err, "describe down")
}

// Test_Package_AWSErrors checks that a read that fails before an update or delete sends anything is reported as
// rejected, and that AWS's errors from the calls that change it are returned.
func Test_Package_AWSErrors(t *testing.T) {
	h, api := newPackageHandler()
	id := createPackage(t, h, dictionaryTemplate())
	request := packageUpdate(t, h, id, withProps(dictionaryTemplate(), map[string]any{"PackageDescription": "x"}))

	api.errs["DescribePackages"] = errors.New("describe down")
	_, err := h.Read(t.Context(), pkgRegion, id)
	require.ErrorContains(t, err, "describe down")
	requirePkgRejectedBefore(t, h.Update(t.Context(), request), "describe down")
	requirePkgRejectedBefore(t, h.Delete(t.Context(), pkgRegion, id), "describe down")
	_, err = h.UpdateState(t.Context(), pkgRegion, id)
	requirePkgRejectedBefore(t, err, "describe down")
	require.Zero(t, api.calls["UpdatePackage"]+api.calls["DeletePackage"])
	delete(api.errs, "DescribePackages")

	api.errs["CreatePackage"] = pkgFault("LimitExceededException", "too many packages")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: withProps(dictionaryTemplate(), map[string]any{"PackageName": "other"})})
	requirePkgRejected(t, err)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: withProps(dictionaryTemplate(), map[string]any{"PackageSource": nil})})
	requirePkgRejectedBefore(t, err, "PackageSource")
	requirePkgRejectedBefore(t, h.Update(t.Context(), native.UpdateRequest{Region: pkgRegion, Identifier: id, Desired: map[string]any{"PackageName": "dictionary", "PackageType": "TXT-DICTIONARY"}}), "PackageSource")

	for _, identifier := range []string{"dictionary", "pkg-XYZ", "F12a"} {
		_, err := h.Read(t.Context(), pkgRegion, identifier)
		requirePkgRejected(t, err)
		requirePkgRejected(t, h.Delete(t.Context(), pkgRegion, identifier))
		_, err = h.Identifier(map[string]any{"PackageID": identifier})
		requirePkgRejected(t, err)
	}
	_, err = h.Identifier(map[string]any{"PackageID": 5})
	requirePkgRejected(t, err)
	_, err = h.Identifier(withProps(dictionaryTemplate(), map[string]any{"PackageName": 5}))
	requirePkgRejectedBefore(t, err, "PackageName")
	resources, err := h.List(t.Context(), pkgRegion)
	require.NoError(t, err)
	require.Empty(t, resources)
}

// pkgOddAnswers is a package API that answers in ways AWS's docs don't promise: a create that returns no package,
// packages in a status the fake never reaches, and, with noFilter, every package whatever the filters.
type pkgOddAnswers struct {
	*pkgFake
	status   types.PackageStatus
	noFilter bool
}

func (o *pkgOddAnswers) CreatePackage(context.Context, *opensearch.CreatePackageInput, ...func(*opensearch.Options)) (*opensearch.CreatePackageOutput, error) {
	return &opensearch.CreatePackageOutput{}, nil
}

func (o *pkgOddAnswers) DescribePackages(ctx context.Context, in *opensearch.DescribePackagesInput, opts ...func(*opensearch.Options)) (*opensearch.DescribePackagesOutput, error) {
	if o.noFilter {
		in = &opensearch.DescribePackagesInput{NextToken: in.NextToken}
	}
	out, err := o.pkgFake.DescribePackages(ctx, in, opts...)
	for i := range out.PackageDetailsList {
		if o.status != "" {
			out.PackageDetailsList[i].PackageStatus = o.status
		}
	}
	return out, err
}

func Test_Package_OddAnswers(t *testing.T) {
	api := newPkgFake(awsfake.NewCloudControl())
	id := createPackage(t, NewPackage(api), dictionaryTemplate())
	o := &pkgOddAnswers{pkgFake: api, status: types.PackageStatusDeleteFailed}
	h := NewPackage(o)

	_, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: dictionaryTemplate()})
	require.ErrorContains(t, err, "returned no ID")

	p, err := h.Progress(t.Context(), pkgRegion, id, native.OperationDelete)
	require.NoError(t, err)
	require.Contains(t, p.Failure.Message, "failed to delete")
	require.NoError(t, h.Delete(t.Context(), pkgRegion, id), "a package whose delete failed is deleted again")

	o.status = types.PackageStatusDeleting
	p, err = h.UpdateProgress(t.Context(), pkgRegion, id, "")
	require.NoError(t, err)
	require.Contains(t, p.Failure.Message, "deleting")

	o.status = types.PackageStatusDeleted
	_, err = h.Progress(t.Context(), pkgRegion, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	// Packages AWS returns that the filter didn't ask for are left out.
	o.status, o.noFilter = "", true
	other := createPackage(t, NewPackage(api), withProps(dictionaryTemplate(), map[string]any{"PackageName": "dictionary-2"}))
	read, err := h.Read(t.Context(), pkgRegion, other)
	require.NoError(t, err)
	require.Equal(t, other, read["PackageID"])
	_, err = h.Read(t.Context(), pkgRegion, "F999999999")
	require.ErrorIs(t, err, native.ErrNotFound)
}

// Test_Package_Plugin checks a plugin with a configuration: AWS gives it the latest engine version when the template
// sets none, and Read leaves out the configuration, which is write-only.
func Test_Package_Plugin(t *testing.T) {
	h, api := newPackageHandler()
	config := map[string]any{"ConfigurationRequirement": "NONE", "LicenseRequirement": "REQUIRED", "LicenseFilepath": "licenses/license.json"}
	properties := map[string]any{"PackageName": "plugin", "PackageType": "ZIP-PLUGIN", "PackageSource": source("plugin.zip"), "PackageConfiguration": config}
	id := createPackage(t, h, properties)
	read, err := h.Read(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	require.Equal(t, "OpenSearch_2.17", read["EngineVersion"])
	require.NotContains(t, read, "PackageConfiguration")
	require.Equal(t, config, api.store.Resource(packageType, id)["PackageConfiguration"])

	api.errs["DescribePackages"] = errors.New("describe down")
	_, err = h.UpdateProgress(t.Context(), pkgRegion, id, "")
	require.ErrorContains(t, err, "describe down")
}

// Test_RegionOptions checks that the shared region options set each client's Region for one call.
func Test_RegionOptions(t *testing.T) {
	opts := opensearch.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
	ec2Opts := awsec2.Options{}
	ec2InRegion("eu-west-3")(&ec2Opts)
	require.Equal(t, "eu-west-3", ec2Opts.Region)
}

// Test_Package_UpdateResent checks that an update UCP sends again, with the state it saved before the first attempt,
// sends nothing when that attempt already reached AWS, and waits for that attempt's import instead of taking a version
// AWS shows before a second import as done.
func Test_Package_UpdateResent(t *testing.T) {
	h, api := newPackageHandler()
	id := createPackage(t, h, dictionaryTemplate())
	request := packageUpdate(t, h, id, withProps(dictionaryTemplate(), map[string]any{"PackageSource": source("synonyms-2.txt")}))
	state, err := h.UpdateState(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	request.State = state
	require.NoError(t, h.Update(t.Context(), request))

	// The response was lost while AWS imported the file. UCP sends the same update again while AWS still imports it.
	for range 3 {
		_, err := h.Read(t.Context(), pkgRegion, id)
		require.NoError(t, err)
		require.NoError(t, h.Update(t.Context(), request))
	}
	require.Equal(t, 1, api.calls["UpdatePackage"])
	p, err := h.UpdateProgress(t.Context(), pkgRegion, id, state)
	require.NoError(t, err)
	for !p.Done {
		p, err = h.UpdateProgress(t.Context(), pkgRegion, id, state)
		require.NoError(t, err)
	}
	require.Nil(t, p.Failure)

	// And once after it finished.
	require.NoError(t, h.Update(t.Context(), request))
	require.Equal(t, 1, api.calls["UpdatePackage"])
	p, err = h.UpdateProgress(t.Context(), pkgRegion, id, state)
	require.NoError(t, err)
	require.True(t, p.Done)
	read, err := h.Read(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	require.Equal(t, "v2", read["AvailablePackageVersion"])

	request.State = "{"
	requirePkgRejectedBefore(t, h.Update(t.Context(), request), "saved version")
	require.Equal(t, 1, api.calls["UpdatePackage"])
}
