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
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// pkgHTTP is an HTTP client that answers each request with the next of its answers: a connection error, or a
// response with a status, an error type, and a body.
type pkgHTTP struct {
	answers []pkgAnswer
	calls   int
}

type pkgAnswer struct {
	err       error
	status    int
	errorType string
	body      string
}

func (c *pkgHTTP) Do(req *http.Request) (*http.Response, error) {
	a := c.answers[min(c.calls, len(c.answers)-1)]
	c.calls++
	if a.err != nil {
		return nil, a.err
	}
	header := http.Header{"Content-Type": []string{"application/json"}}
	if a.errorType != "" {
		header.Set("X-Amzn-Errortype", a.errorType)
	}
	return &http.Response{StatusCode: a.status, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(a.body))), Request: req}, nil
}

// sdkPackageAPI returns the real SDK client, with its default retries, answering with answers.
func sdkPackageAPI(answers ...pkgAnswer) (*opensearch.Client, *pkgHTTP) {
	c := &pkgHTTP{answers: answers}
	return opensearch.New(opensearch.Options{Region: pkgRegion, Credentials: aws.AnonymousCredentials{}, HTTPClient: c}), c
}

// Test_Package_NoSDKRetry checks that the SDK doesn't send a change again on its own after a connection error, because
// a first attempt that reached AWS would make a retried create fail with ResourceAlreadyExistsException, which reads as
// a create AWS rejected, and a retried update or association import the file or deploy the domain again.
func Test_Package_NoSDKRetry(t *testing.T) {
	reset := pkgAnswer{err: errors.New("read tcp 10.0.0.1:443: connection reset by peer")}
	exists := pkgAnswer{status: http.StatusConflict, errorType: "ResourceAlreadyExistsException", body: `{"message":"exists"}`}
	client, c := sdkPackageAPI(reset, exists)
	h := NewPackage(client, newPkgFake(awsfake.NewCloudControl()))
	_, err := h.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: dictionaryTemplate()})
	require.Equal(t, 1, c.calls, "the SDK sent the create again")
	var failure *native.Failure
	require.False(t, errors.As(err, &failure), "AWS may have taken the create, so it isn't rejected: %v", err)

	described := pkgAnswer{status: http.StatusOK, body: `{"PackageDetailsList":[{"PackageID":"F123456789","PackageStatus":"AVAILABLE","AvailablePackageVersion":"v1"}]}`}
	client, c = sdkPackageAPI(described, reset, described)
	h = NewPackage(client, newPkgFake(awsfake.NewCloudControl()))
	err = h.Update(t.Context(), native.UpdateRequest{Region: pkgRegion, Identifier: "F123456789", Desired: dictionaryTemplate()})
	require.Error(t, err)
	require.Equal(t, 2, c.calls, "the SDK sent the update again")

	client, c = sdkPackageAPI(pkgAnswer{status: http.StatusOK, body: `{"DomainPackageDetailsList":[]}`}, reset, described)
	a := NewPackageAssociation(client)
	_, err = a.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: map[string]any{"DomainName": "logs", "PackageID": "F123456789"}})
	require.Error(t, err)
	require.Equal(t, 2, c.calls, "the SDK sent the association again")
}

// Test_Package_FindMismatchKeepsNoID checks that Find, and an association create that finds an association with
// other values, return no identifier with their error, because UCP keeps an identifier returned with an error, and a
// later DELETE would delete what it names. An empty prerequisite list AWS returns is a value, not a missing one.
func Test_Package_FindMismatchKeepsNoID(t *testing.T) {
	h, _ := newPackageHandler()
	pluginPackage := map[string]any{"PackageName": "plugin", "PackageType": "ZIP-PLUGIN", "PackageSource": source("plugin.zip")}
	createPackage(t, h, withProps(pluginPackage, map[string]any{"EngineVersion": "OpenSearch_2.15"}))
	createPackage(t, h, withProps(pluginPackage, map[string]any{"EngineVersion": "OpenSearch_2.17"}))
	id, err := h.Find(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: map[string]any{"PackageName": "plugin", "PackageType": "ZIP-PLUGIN"}})
	require.ErrorContains(t, err, "match the interrupted create")
	require.Empty(t, id)

	a, aapi := newAssociationHandler()
	aapi.echoOptional = true
	createAssociation(t, a, pluginTemplate)
	createAssociation(t, a, licenseTemplate)
	id, err = a.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: map[string]any{"DomainName": "logs", "PackageID": license}})
	requirePkgRejectedBefore(t, err, "other PrerequisitePackageIDList")
	require.Empty(t, id)

	createAssociation(t, a, map[string]any{"DomainName": "logs", "PackageID": "F111111111", "PrerequisitePackageIDList": []any{}})
	_, err = a.Create(t.Context(), native.CreateRequest{Region: pkgRegion, Properties: map[string]any{
		"DomainName": "logs", "PackageID": "F111111111", "PrerequisitePackageIDList": []any{plugin},
	}})
	requirePkgRejectedBefore(t, err, "other PrerequisitePackageIDList")
}

// Test_Package_UpdateAfterOutsideUpdate checks that an update whose package someone else updated after UCP saved its
// state still imports the template's file, and waits for its own version, instead of taking the other one as its own.
func Test_Package_UpdateAfterOutsideUpdate(t *testing.T) {
	h, api := newPackageHandler()
	id := createPackage(t, h, dictionaryTemplate())
	request := packageUpdate(t, h, id, withProps(dictionaryTemplate(), map[string]any{"PackageSource": source("synonyms-2.txt")}))
	state, err := h.UpdateState(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	request.State = state

	// Outside Radius, someone imports another file, and AWS is still importing it.
	_, err = api.UpdatePackage(t.Context(), &opensearch.UpdatePackageInput{PackageID: aws.String(id), PackageSource: &types.PackageSource{
		S3BucketName: aws.String(pkgBucket), S3Key: aws.String("stopwords.txt"),
	}})
	require.NoError(t, err)
	for range 2 {
		_, err := h.Read(t.Context(), pkgRegion, id)
		require.NoError(t, err)
	}
	requirePkgConflict(t, h.Update(t.Context(), request), "still importing")
	require.Equal(t, 1, api.calls["UpdatePackage"])

	// Once it is done, the update imports the template's file and waits for that version.
	require.NoError(t, settlePackages(t.Context(), api))
	require.NoError(t, h.Update(t.Context(), request))
	require.Equal(t, 2, api.calls["UpdatePackage"])
	p, err := h.UpdateProgress(t.Context(), pkgRegion, id, state)
	require.NoError(t, err)
	require.False(t, p.Done, "the outside version isn't this update's")
	require.Nil(t, waitUpdate(t, h, id, state).Failure)
	require.Equal(t, source("synonyms-2.txt"), api.store.Resource(packageType, id)["PackageSource"])
}

// Test_Package_UpdateReplacedOutside checks that an update another one replaced before UCP saw it finish fails, so
// UCP doesn't record the template's file as what AWS has.
func Test_Package_UpdateReplacedOutside(t *testing.T) {
	h, api := newPackageHandler()
	id := createPackage(t, h, dictionaryTemplate())
	request := packageUpdate(t, h, id, withProps(dictionaryTemplate(), map[string]any{"PackageSource": source("synonyms-2.txt")}))
	state, err := h.UpdateState(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	request.State = state
	require.NoError(t, h.Update(t.Context(), request))
	require.NoError(t, settlePackages(t.Context(), api))
	_, err = api.UpdatePackage(t.Context(), &opensearch.UpdatePackageInput{PackageID: aws.String(id), PackageSource: &types.PackageSource{
		S3BucketName: aws.String(pkgBucket), S3Key: aws.String("stopwords.txt"),
	}})
	require.NoError(t, err)
	require.NoError(t, settlePackages(t.Context(), api))

	p, err := h.UpdateProgress(t.Context(), pkgRegion, id, state)
	require.NoError(t, err)
	require.True(t, p.Done)
	require.NotNil(t, p.Failure)
	require.Contains(t, p.Failure.Message, "another update")
}

// Test_Package_HistoryFails checks that an update sent again, and the wait for an update, fail when AWS can't list the
// package's versions, and that the update sends nothing then.
func Test_Package_HistoryFails(t *testing.T) {
	h, api := newPackageHandler()
	id := createPackage(t, h, dictionaryTemplate())
	request := packageUpdate(t, h, id, withProps(dictionaryTemplate(), map[string]any{"PackageSource": source("synonyms-2.txt")}))
	state, err := h.UpdateState(t.Context(), pkgRegion, id)
	require.NoError(t, err)
	request.State = state
	require.NoError(t, h.Update(t.Context(), request))
	require.NoError(t, settlePackages(t.Context(), api))

	api.errs["GetPackageVersionHistory"] = errors.New("history down")
	requirePkgRejectedBefore(t, h.Update(t.Context(), request), "history down")
	_, err = h.UpdateProgress(t.Context(), pkgRegion, id, state)
	require.ErrorContains(t, err, "history down")
	require.Equal(t, 1, api.calls["UpdatePackage"])
}

// pkgHistoryTimes is a package API whose version history leaves out or repeats CreatedAt, which the docs make optional.
type pkgHistoryTimes struct {
	*pkgFake
	edit func(list []types.PackageVersionHistory)
}

func (p pkgHistoryTimes) GetPackageVersionHistory(ctx context.Context, in *opensearch.GetPackageVersionHistoryInput, opts ...func(*opensearch.Options)) (*opensearch.GetPackageVersionHistoryOutput, error) {
	out, err := p.pkgFake.GetPackageVersionHistory(ctx, in, opts...)
	if err == nil {
		out.PackageVersionHistoryList = slices.Clone(out.PackageVersionHistoryList)
		p.edit(out.PackageVersionHistoryList)
	}
	return out, err
}

// Test_Package_UpdateHistoryTimes checks that an update isn't done when the version history can't show its version is
// the newest: another version without CreatedAt, or with the same one.
func Test_Package_UpdateHistoryTimes(t *testing.T) {
	for name, edit := range map[string]func(list []types.PackageVersionHistory){
		"other version without CreatedAt": func(list []types.PackageVersionHistory) { list[0].CreatedAt = nil },
		"other version at the same time":  func(list []types.PackageVersionHistory) { list[0].CreatedAt = list[len(list)-1].CreatedAt },
		"own version without CreatedAt":   func(list []types.PackageVersionHistory) { list[len(list)-1].CreatedAt = nil },
	} {
		t.Run(name, func(t *testing.T) {
			api := newPkgFake(awsfake.NewCloudControl())
			h := NewPackage(pkgHistoryTimes{pkgFake: api, edit: edit}, api)
			id := createPackage(t, h, dictionaryTemplate())
			api.versions[id], api.history[id] = 1, api.history[id][:1]
			p := updatePackage(t, h, packageUpdate(t, h, id, withProps(dictionaryTemplate(), map[string]any{"PackageSource": source("synonyms-2.txt")})))
			require.NotNil(t, p.Failure)
			require.Contains(t, p.Failure.Message, "newest")
		})
	}
}

// Test_Suite_PackageEmptyDescription runs the suite with an empty description, which AWS returns empty or not at all,
// so its redeploy cases check that an unchanged template changes nothing.
func Test_Suite_PackageEmptyDescription(t *testing.T) {
	properties := withProps(dictionaryTemplate(), map[string]any{"PackageDescription": ""})
	nativetest.Run(t, nativetest.Suite{
		TypeName:    packageType,
		Schema:      readSchemaFile(t, "aws-opensearchservice-package.json"),
		Metadata:    readSchemaFile(t, "aws-opensearchservice-package.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newPkgFake(store)
			return NewPackage(api, api)
		},
		IdentifierProperty: "PackageID",
		WriteOnly:          []string{"PackageSource", "PackageConfiguration", "PackageEncryptionOptions", "PackageVendingOptions"},
		Properties:         properties,
		Changed:            withProps(properties, map[string]any{"PackageDescription": "Synonyms"}),
		API:                packageAPI,
	})
}
