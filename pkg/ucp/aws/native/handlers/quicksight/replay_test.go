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

package quicksight

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/quicksight"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// requireFailure checks that err is a client failure AWS applied none of, and whether it is a conflict.
func requireFailure(t *testing.T, err error, conflict bool) *native.Failure {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError, "%v", err)
	require.Equal(t, conflict, failure.Conflict, "%v", err)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.False(t, errors.Is(err, native.ErrNotFound))
	return failure
}

// Test_Replay_FolderMembership adds a dashboard to a folder and removes it, against AWS's responses. Every property
// is create-only, so there is no update step.
func Test_Replay_FolderMembership(t *testing.T) {
	s := awsreplay.New(t, "testdata/foldermembership.json")
	properties := map[string]any{
		"AwsAccountId": s.Input("RADIUS_REC_ACCOUNT_ID"),
		"FolderId":     s.Input("RADIUS_REC_QUICKSIGHT_FOLDER_ID"),
		"MemberType":   "DASHBOARD",
		"MemberId":     s.Input("RADIUS_REC_QUICKSIGHT_DASHBOARD_ID"),
	}
	nativetest.Lifecycle{
		Handler:    NewFolderMembership(quicksight.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: properties,
		Poll:       5 * time.Second,
	}.Run(t)
}

// Test_Replay_FolderMembershipErrors runs AWS's errors through the SDK: Quick Sight's 401 AccessDeniedException must be
// rejected, so UCP answers 400, not 500; ResourceExistsException means the folder already has the asset; a
// ConflictException, which the SDK's model doesn't list for DeleteFolderMembership, must be a conflict; and a folder
// that doesn't exist reads as not found.
func Test_Replay_FolderMembershipErrors(t *testing.T) {
	s := awsreplay.New(t, "testdata/foldermembershiperrors.json")
	h := NewFolderMembership(quicksight.NewFromConfig(s.Config(t.Context())))
	properties := map[string]any{
		"AwsAccountId": s.Input("RADIUS_REC_ACCOUNT_ID"),
		"FolderId":     s.Input("RADIUS_REC_QUICKSIGHT_FOLDER_ID"),
		"MemberType":   "DASHBOARD",
		"MemberId":     s.Input("RADIUS_REC_QUICKSIGHT_DASHBOARD_ID"),
	}

	_, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: properties})
	require.Contains(t, requireFailure(t, err, false).Message, "AccessDeniedException")

	id, err := h.Create(t.Context(), native.CreateRequest{Region: awsreplay.Region, Properties: properties})
	require.NoError(t, err)
	require.Equal(t, membershipTemplateID(properties), id)

	require.Contains(t, requireFailure(t, h.Delete(t.Context(), awsreplay.Region, id), true).Message, "dependsOn")

	_, err = h.Read(t.Context(), awsreplay.Region, properties["AwsAccountId"].(string)+"|radius-rec-missing|DASHBOARD|x")
	require.ErrorIs(t, err, native.ErrNotFound)
}

func membershipTemplateID(properties map[string]any) string {
	id, _, _ := parseMembership(properties)
	return id.String()
}

// Test_Replay_TemplateAlias creates an alias of a template and points it to another version, against AWS's
// responses. A delete leaves the alias in AWS, so there is no delete step.
func Test_Replay_TemplateAlias(t *testing.T) {
	s := awsreplay.New(t, "testdata/templatealias.json")
	properties := map[string]any{
		"AwsAccountId":          s.Input("RADIUS_REC_ACCOUNT_ID"),
		"TemplateId":            s.Input("RADIUS_REC_QUICKSIGHT_TEMPLATE_ID"),
		"AliasName":             "live",
		"TemplateVersionNumber": float64(1),
	}
	update := with(properties, map[string]any{"TemplateVersionNumber": float64(2)})
	nativetest.Lifecycle{
		Handler:    NewTemplateAlias(quicksight.NewFromConfig(s.Config(t.Context()))),
		Session:    s,
		Properties: properties,
		Update:     update,
		Poll:       5 * time.Second,
		SkipDelete: true,
	}.Run(t)
}

// Test_Replay_TemplateAliasErrors runs AWS's errors through the SDK: ConflictException must be a conflict;
// ResourceExistsException points the alias that is already there to the template's version; ResourceNotFoundException
// from an update, which AWS also returns for a version the template doesn't have, must be rejected, not a missing
// alias; UnsupportedUserEditionException must be rejected; and a missing alias reads as not found.
func Test_Replay_TemplateAliasErrors(t *testing.T) {
	s := awsreplay.New(t, "testdata/templatealiaserrors.json")
	h := NewTemplateAlias(quicksight.NewFromConfig(s.Config(t.Context())))
	properties := map[string]any{
		"AwsAccountId":          s.Input("RADIUS_REC_ACCOUNT_ID"),
		"TemplateId":            s.Input("RADIUS_REC_QUICKSIGHT_TEMPLATE_ID"),
		"AliasName":             "live",
		"TemplateVersionNumber": float64(2),
	}
	create := native.CreateRequest{Region: awsreplay.Region, Properties: properties}

	_, err := h.Create(t.Context(), create)
	require.Contains(t, requireFailure(t, err, true).Message, "dependsOn")

	id, err := h.Create(t.Context(), create)
	require.NoError(t, err)

	err = h.Update(t.Context(), native.UpdateRequest{Region: awsreplay.Region, Identifier: id, Desired: with(properties, map[string]any{"TemplateVersionNumber": float64(9)})})
	require.Contains(t, requireFailure(t, err, false).Message, "ResourceNotFoundException")

	_, err = h.Create(t.Context(), create)
	require.Contains(t, requireFailure(t, err, false).Message, "UnsupportedUserEditionException")

	_, err = h.Read(t.Context(), awsreplay.Region, properties["AwsAccountId"].(string)+"|"+properties["TemplateId"].(string)+"|gone")
	require.ErrorIs(t, err, native.ErrNotFound)
}
