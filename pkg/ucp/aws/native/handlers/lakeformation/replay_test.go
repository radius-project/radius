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

package lakeformation

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_DataLakeSettings adds an admin with MutationType APPEND, then sets AuthorizedSessionTagValueList,
// against AWS's recorded responses. A delete changes nothing in AWS, so the test puts back the settings it found.
func Test_Replay_DataLakeSettings(t *testing.T) {
	s := awsreplay.New(t, "testdata/datalakesettings.json")
	api := lakeformation.NewFromConfig(s.Config(t.Context()))
	run := s.Input("RADIUS_REC_RUN")
	admin := s.Input("RADIUS_REC_LAKE_PRINCIPAL_ARN")

	original, err := api.GetDataLakeSettings(t.Context(), &lakeformation.GetDataLakeSettingsInput{})
	require.NoError(t, err)
	t.Cleanup(func() {
		// t.Context() ends before cleanups run.
		_, err := api.PutDataLakeSettings(context.WithoutCancel(t.Context()), &lakeformation.PutDataLakeSettingsInput{DataLakeSettings: original.DataLakeSettings})
		if err != nil {
			t.Errorf("failed to put back the data lake settings: %v", err)
		}
	})

	properties := map[string]any{"MutationType": "APPEND", "Admins": admins(admin)}
	update := with(properties, map[string]any{"AuthorizedSessionTagValueList": list("radius-rec-" + run)})
	nativetest.Lifecycle{
		Handler:     NewDataLakeSettings(api),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"MutationType": nil, "Id": settingsIdentifier}),
		Update:      update,
		WantUpdated: with(update, map[string]any{"MutationType": nil}),
		SkipDelete:  true,
	}.Run(t)
}

// Test_Replay_Permissions grants a permission on a Glue database, replaces it with another, and revokes it, against
// AWS's recorded responses.
func Test_Replay_Permissions(t *testing.T) {
	s := awsreplay.New(t, "testdata/permissions.json")
	api := lakeformation.NewFromConfig(s.Config(t.Context()))
	database := map[string]any{"DatabaseResource": map[string]any{"Name": s.Input("RADIUS_REC_GLUE_DATABASE")}}
	properties := map[string]any{
		"DataLakePrincipal": map[string]any{"DataLakePrincipalIdentifier": s.Input("RADIUS_REC_LAKE_PRINCIPAL_ARN")},
		"Resource":          database,
		"Permissions":       list("ALTER"),
	}
	nativetest.Lifecycle{
		Handler:    NewPermissions(api),
		Session:    s,
		Properties: properties,
		Update:     with(properties, map[string]any{"Permissions": list("DESCRIBE")}),
	}.Run(t)
}

// Test_Replay_Resource registers an S3 location with a role, turns on hybrid access, and deregisters it, against AWS's
// recorded responses.
func Test_Replay_Resource(t *testing.T) {
	s := awsreplay.New(t, "testdata/resource.json")
	api := lakeformation.NewFromConfig(s.Config(t.Context()))
	arn := "arn:aws:s3:::" + s.Input("RADIUS_REC_LAKE_BUCKET") + "/radius-rec-" + s.Input("RADIUS_REC_RUN")
	properties := map[string]any{"ResourceArn": arn, "RoleArn": s.Input("RADIUS_REC_LAKE_ROLE_ARN"), "UseServiceLinkedRole": false}
	update := with(properties, map[string]any{"HybridAccessEnabled": true})
	nativetest.Lifecycle{
		Handler:     NewResource(api),
		Session:     s,
		Properties:  properties,
		Want:        with(properties, map[string]any{"UseServiceLinkedRole": nil, "Id": arn}),
		Update:      update,
		WantUpdated: with(update, map[string]any{"UseServiceLinkedRole": nil}),
	}.Run(t)
}
