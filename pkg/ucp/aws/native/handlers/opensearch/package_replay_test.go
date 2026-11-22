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
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// requireReplayFailure checks that err is a failure AWS applied none of, a conflict when conflict is true, and that
// it names AWS's error code.
func requireReplayFailure(t *testing.T, err error, conflict bool, code string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, conflict, failure.Conflict, "%v", err)
	require.True(t, failure.ClientError)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, code)
	require.NotErrorIs(t, err, native.ErrNotFound)
}

// Test_Replay_Package imports a dictionary package from S3, updates its description and file, which makes a new
// version, and deletes it, against AWS's responses.
func Test_Replay_Package(t *testing.T) {
	s := awsreplay.New(t, "testdata/package.json")
	cfg := s.Config(t.Context())
	bucket := s.Input("RADIUS_REC_PACKAGE_BUCKET")
	properties := map[string]any{
		"PackageName":        "radius-rec-synonyms",
		"PackageType":        "TXT-DICTIONARY",
		"PackageSource":      map[string]any{"S3BucketName": bucket, "S3Key": "synonyms.txt"},
		"PackageDescription": "Radius recording",
	}
	update := withProps(properties, map[string]any{
		"PackageSource":      map[string]any{"S3BucketName": bucket, "S3Key": "synonyms-2.txt"},
		"PackageDescription": "Radius recording 2",
	})
	without := func(p map[string]any, changes map[string]any) map[string]any {
		out := withProps(p, changes)
		delete(out, "PackageSource")
		return out
	}
	nativetest.Lifecycle{
		Handler:     NewPackage(opensearch.NewFromConfig(cfg), sts.NewFromConfig(cfg)),
		Session:     s,
		Properties:  properties,
		Want:        without(properties, map[string]any{"PackageStatus": "AVAILABLE", "AvailablePackageVersion": "v1"}),
		Update:      update,
		WantUpdated: without(update, map[string]any{"PackageStatus": "AVAILABLE", "AvailablePackageVersion": "v2"}),
		Poll:        10 * time.Second,
	}.Run(t)
}

// Test_Replay_PackageErrors runs AWS's errors through the SDK: a create of a name AWS already has, 409
// ResourceAlreadyExistsException, and an update AWS answers with 400 ValidationException, must be rejected, so UCP
// answers 400; a delete of a package a domain uses, 409 ConflictException, must be a conflict; and a read AWS answers
// with 400 ValidationException "Package not found", as other AWS clients see, must be not found. An update sent again reads the package's version history.
func Test_Replay_PackageErrors(t *testing.T) {
	s := awsreplay.New(t, "testdata/packageerrors.json")
	cfg := s.Config(t.Context())
	h := NewPackage(opensearch.NewFromConfig(cfg), sts.NewFromConfig(cfg))
	bucket := s.Input("RADIUS_REC_PACKAGE_BUCKET")
	properties := map[string]any{
		"PackageName":   "radius-rec-synonyms",
		"PackageType":   "TXT-DICTIONARY",
		"PackageSource": map[string]any{"S3BucketName": bucket, "S3Key": "synonyms.txt"},
	}

	_, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: properties})
	requireReplayFailure(t, err, false, "ResourceAlreadyExistsException")

	requireReplayFailure(t, h.Delete(t.Context(), awsreplay.Region, "F123456789"), true, "dependsOn")

	err = h.Update(t.Context(), native.UpdateRequest{
		Region:     awsreplay.Region,
		Identifier: "F123456789",
		Desired:    withProps(properties, map[string]any{"PackageSource": map[string]any{"S3BucketName": bucket, "S3Key": "missing.txt"}}),
		Patch:      jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/PackageSource/S3Key", Value: "missing.txt"}},
	})
	requireReplayFailure(t, err, false, "ValidationException")

	_, err = h.Read(t.Context(), awsreplay.Region, "F999999999")
	require.ErrorIs(t, err, native.ErrNotFound)

	// An update sent again after its first attempt made version v2 finds that version by its commit message in the
	// package's history, and sends nothing.
	err = h.Update(t.Context(), native.UpdateRequest{
		Region:     awsreplay.Region,
		Identifier: "F123456789",
		Desired:    withProps(properties, map[string]any{"PackageSource": map[string]any{"S3BucketName": bucket, "S3Key": "synonyms-2.txt"}}),
		State:      `{"version":"v1","status":"AVAILABLE","updated":"2026-11-20T00:00:00Z","commit":"0b9f1c2e-6a4d-4c1b-9e7f-3d2a1b0c9e8f"}`,
	})
	require.NoError(t, err)
}

// Test_Replay_PackageAssociation associates a dictionary package with a domain and dissociates it, against AWS's
// responses. Every property is create-only, so there is no update step.
func Test_Replay_PackageAssociation(t *testing.T) {
	s := awsreplay.New(t, "testdata/packageassociation.json")
	properties := map[string]any{
		"DomainName": s.Input("RADIUS_REC_OPENSEARCH_DOMAIN"),
		"PackageID":  s.Input("RADIUS_REC_PACKAGE_ID"),
	}
	nativetest.Lifecycle{
		Handler:    NewPackageAssociation(opensearch.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: properties,
		Want:       withProps(properties, map[string]any{"DomainPackageStatus": "ACTIVE", "PackageType": "TXT-DICTIONARY"}),
		Poll:       10 * time.Second,
	}.Run(t)
}

// Test_Replay_PackageAssociationErrors runs AWS's errors through the SDK: an association while AWS is changing the
// domain, 409 ConflictException, must be a conflict; one with a domain that doesn't exist, 409
// ResourceNotFoundException, must be rejected, not not found, while a read of it is not found; and a dissociation AWS
// refuses because the package is in use must be a conflict, and one AWS answers with 400 ValidationException because
// the package is no longer associated is not found.
func Test_Replay_PackageAssociationErrors(t *testing.T) {
	s := awsreplay.New(t, "testdata/packageassociationerrors.json")
	h := NewPackageAssociation(opensearch.NewFromConfig(s.Config(t.Context())))
	domain, packageID := s.Input("RADIUS_REC_OPENSEARCH_DOMAIN"), s.Input("RADIUS_REC_PACKAGE_ID")

	// AWS rejected both creates, so neither returns an identifier.
	created, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"DomainName": domain, "PackageID": packageID}})
	requireReplayFailure(t, err, true, "dependsOn")
	require.Empty(t, created)

	created, err = h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: map[string]any{"DomainName": "radius-rec-missing", "PackageID": packageID}})
	requireReplayFailure(t, err, false, "ResourceNotFoundException")
	require.Empty(t, created)

	_, err = h.Read(t.Context(), awsreplay.Region, "radius-rec-missing|"+packageID)
	require.ErrorIs(t, err, native.ErrNotFound)

	requireReplayFailure(t, h.Delete(t.Context(), awsreplay.Region, domain+"|"+packageID), true, "index uses the package")

	// A package dissociated since the list, which AWS answers with 400 ValidationException, is gone.
	require.ErrorIs(t, h.Delete(t.Context(), awsreplay.Region, domain+"|"+packageID), native.ErrNotFound)
}
