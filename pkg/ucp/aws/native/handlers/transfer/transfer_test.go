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

package transfer

import (
	"encoding/json"
	"maps"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/transfer"
	"github.com/aws/aws-sdk-go-v2/service/transfer/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region     = "us-west-2"
	identifier = server + "|" + externalID
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

var access = map[string]any{
	"ServerId":      server,
	"ExternalId":    externalID,
	"Role":          role,
	"HomeDirectory": "/bucket/home",
	"Policy":        `{"Version":"2012-10-17","Statement":[]}`,
	"PosixProfile":  map[string]any{"Uid": float64(1000), "Gid": float64(100), "SecondaryGids": []any{float64(4)}},
}

var mappings = []any{map[string]any{"Entry": "/", "Target": "/bucket/home", "Type": "DIRECTORY"}}

func newHandler() (*Access, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewAccess(api), api
}

// update reads the access and sends an update to desired, with the patch UCP computes from AWS's values, the
// template, and the last deploy, lastApplied.
func update(t *testing.T, h *Access, lastApplied map[string]any, desired map[string]any) error {
	current, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	patch := patchOf(t, current, lastApplied, desired)
	return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: desired, Patch: patch})
}

// patchOf returns the patch UCP computes from AWS's values, current, to the template, desired, after a deploy of
// lastApplied, when Role is the one the last deploy sent. UCP keeps no write-only value, and leaves out of the patch
// one whose fingerprint matches, so neither side has Role. The suite covers a changed Role through UCP itself.
func patchOf(t *testing.T, current map[string]any, lastApplied map[string]any, desired map[string]any) jsondiff.Patch {
	if lastApplied != nil {
		lastApplied = with(lastApplied, map[string]any{"Role": nil})
	}
	desired = with(desired, map[string]any{"Role": nil})
	c, err := json.Marshal(current)
	require.NoError(t, err)
	d, err := json.Marshal(desired)
	require.NoError(t, err)
	removed := awsoperations.RemovedPaths(lastApplied, desired)
	patch, err := awsoperations.GeneratePatchWithOptions(c, d, []byte(readFile(t, "aws-transfer-access.json")), awsoperations.PatchOptions{LastApplied: lastApplied, Removed: removed, Equivalent: samePolicy})
	require.NoError(t, err)
	return patch
}

func Test_Suite_Access(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    accessType,
		Schema:      readFile(t, "aws-transfer-access.json"),
		Metadata:    readFile(t, "aws-transfer-access.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewAccess(newFakeAPI(store))
		},
		Properties:       access,
		Identifier:       identifier,
		WriteOnly:        []string{"Role"},
		ChangedWriteOnly: with(access, map[string]any{"Role": "arn:aws:iam::123456789012:role/transfer-other", "HomeDirectoryType": "LOGICAL", "HomeDirectoryMappings": mappings, "HomeDirectory": nil}),
		OtherForm:        map[string]any{"Policy": `{ "Statement": [], "Version": "2012-10-17" }`},
		Changed:          with(access, map[string]any{"HomeDirectoryType": "LOGICAL", "HomeDirectoryMappings": mappings, "HomeDirectory": nil}),
		Removable:        "Policy",
		CreateOnlyChange: with(access, map[string]any{"ExternalId": "S-1-1-12-99"}),
		Invalid: []map[string]any{
			with(access, map[string]any{"ServerId": "s-0123|x"}),
			with(access, map[string]any{"ExternalId": 5}),
			with(access, map[string]any{"Policy": map[string]any{"Version": "2012-10-17"}}),
			with(access, map[string]any{"HomeDirectoryMappings": "/bucket"}),
			with(access, map[string]any{"HomeDirectoryMappings": []any{"/bucket"}}),
			with(access, map[string]any{"HomeDirectoryMappings": []any{map[string]any{"Entry": "/"}}}),
			with(access, map[string]any{"HomeDirectoryMappings": []any{map[string]any{"Entry": "/", "Target": "/b", "Type": 1}}}),
			with(access, map[string]any{"PosixProfile": "1000:100"}),
			with(access, map[string]any{"PosixProfile": map[string]any{"Uid": 1.5, "Gid": float64(100)}}),
			with(access, map[string]any{"PosixProfile": map[string]any{"Uid": float64(1000), "Gid": float64(-1)}}),
			with(access, map[string]any{"PosixProfile": map[string]any{"Uid": float64(1000), "Gid": float64(100), "SecondaryGids": float64(4)}}),
			with(access, map[string]any{"PosixProfile": map[string]any{"Uid": float64(1000), "Gid": float64(100), "SecondaryGids": []any{"4"}}}),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			id, err := parseID(identifier)
			require.NoError(t, err)
			return map[string]any{"ServerId": id.server, "ExternalId": id.external, "Role": role}
		},
		API: accessAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := transfer.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_Identifier(t *testing.T) {
	h, _ := newHandler()
	id, err := h.Identifier(access)
	require.NoError(t, err)
	require.Equal(t, identifier, id)

	id, err = h.Identifier(with(access, map[string]any{"ExternalId": nil}))
	require.NoError(t, err)
	require.Empty(t, id, "an access without an ExternalId has no identifier")

	var failure *native.Failure
	_, err = h.Identifier(with(access, map[string]any{"ExternalId": "S-1|2"}))
	require.ErrorAs(t, err, &failure)
	require.Contains(t, failure.Message, `ExternalId of an AWS::Transfer::Access can't contain "|"`)
	_, err = h.Identifier(with(access, map[string]any{"Role": 7}))
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "Role of an AWS::Transfer::Access must be a string", failure.Message)
}

func Test_BadIdentifier(t *testing.T) {
	h, api := newHandler()
	for _, id := range []string{server, "|" + externalID, server + "|", server + "|a|b"} {
		var failure *native.Failure
		_, err := h.Read(t.Context(), region, id)
		require.ErrorAs(t, err, &failure, id)
		require.True(t, failure.ClientError)
		require.ErrorAs(t, h.Delete(t.Context(), region, id), &failure)
		_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
		require.ErrorAs(t, err, &failure)
		require.ErrorAs(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Desired: access}), &failure)
		require.Contains(t, failure.Message, "is not the identifier of an AWS Transfer Family access")
	}
	require.Zero(t, api.updates)
}

func Test_CreateAndRead(t *testing.T) {
	h, api := newHandler()
	var failure *native.Failure
	for _, name := range []string{"ServerId", "Role"} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(access, map[string]any{name: nil})})
		require.ErrorAs(t, err, &failure, name)
		require.Equal(t, "an AWS::Transfer::Access needs ServerId, ExternalId, and Role", failure.Message)
		require.Equal(t, native.OutcomeRejected, failure.Outcome, "AWS never saw the create")
	}
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(access, map[string]any{"Policy": 1})})
	require.ErrorAs(t, err, &failure)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(access, map[string]any{"HomeDirectoryType": "LOGICAL"})})
	require.ErrorContains(t, err, invalidRequest, "AWS's error is returned")
	require.Zero(t, api.store.Count(accessType))

	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(access, map[string]any{
		"HomeDirectoryType":     "LOGICAL",
		"HomeDirectoryMappings": []any{map[string]any{"Entry": "/", "Target": "/bucket/home"}},
		"PosixProfile":          map[string]any{"Uid": float64(0), "Gid": float64(4294967295)},
	})})
	require.NoError(t, err)
	require.Equal(t, identifier, id)
	properties, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, with(access, map[string]any{
		"Role":                  nil,
		"HomeDirectoryType":     "LOGICAL",
		"HomeDirectoryMappings": mappings,
		"PosixProfile":          map[string]any{"Uid": float64(0), "Gid": float64(4294967295), "SecondaryGids": []any{}},
	}), properties, "AWS fills in a mapping's Type, a profile without secondary groups has an empty list, and Role isn't read")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: access})
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "an earlier attempt may have made the access, so UCP keeps the record")
	require.Contains(t, failure.Message, "ResourceExistsException")
}

func Test_ReadFails(t *testing.T) {
	h, api := newHandler()
	_, err := h.Read(t.Context(), region, identifier)
	require.ErrorIs(t, err, native.ErrNotFound)
	api.errs["DescribeAccess"] = fault("ThrottlingException")
	_, err = h.Read(t.Context(), region, identifier)
	require.ErrorContains(t, err, "ThrottlingException")
	require.NotErrorIs(t, err, native.ErrNotFound)

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, resources)
}

// Test_UpdateRemovals checks what a deploy that removes a property sends: an empty HomeDirectory or Policy, PATH for
// HomeDirectoryType, and nothing for a property AWS can't remove, which fails before any call.
func Test_UpdateRemovals(t *testing.T) {
	h, api := newHandler()
	start := with(access, map[string]any{"HomeDirectoryType": "LOGICAL", "HomeDirectoryMappings": mappings})
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: start})
	require.NoError(t, err)

	for _, name := range []string{"HomeDirectoryMappings", "PosixProfile"} {
		var failure *native.Failure
		require.ErrorAs(t, update(t, h, start, with(start, map[string]any{name: nil})), &failure, name)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
		require.Contains(t, failure.Message, "AWS can't remove "+name+" from a Transfer Family access")
	}
	require.Zero(t, api.updates, "nothing is sent for a removal AWS can't make")

	cleared := with(start, map[string]any{"HomeDirectory": nil, "Policy": nil})
	require.NoError(t, update(t, h, start, cleared))
	require.Equal(t, aws.String(""), api.lastUpdate.HomeDirectory)
	require.Equal(t, aws.String(""), api.lastUpdate.Policy)
	require.Nil(t, api.lastUpdate.Role, "a Role the fingerprint shows unchanged isn't sent")
	current, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, "", current["HomeDirectory"])
	require.Equal(t, "", current["Policy"])
	require.Empty(t, patchOf(t, current, cleared, cleared), "a redeploy after the removal sends nothing")

	// AWS keeps the mappings with PATH, so the template has to keep them too.
	require.NoError(t, update(t, h, cleared, with(cleared, map[string]any{"HomeDirectoryType": nil})))
	require.Equal(t, types.HomeDirectoryTypePath, api.lastUpdate.HomeDirectoryType)
	require.Nil(t, api.lastUpdate.HomeDirectoryMappings)
}

func Test_UpdateSends(t *testing.T) {
	h, api := newHandler()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: access})
	require.NoError(t, err)

	require.NoError(t, update(t, h, access, access), "an update with no change")
	require.NoError(t, update(t, h, access, with(access, map[string]any{"Tags": []any{}})), "an update of a property AWS doesn't have")
	require.Zero(t, api.updates, "an update that changes nothing AWS has sends nothing")

	desired := with(access, map[string]any{
		"Policy":       `{"Version":"2012-10-17","Statement":[{"Effect":"Allow"}]}`,
		"PosixProfile": map[string]any{"Uid": float64(1000), "Gid": float64(100)},
	})
	require.NoError(t, update(t, h, access, desired))
	require.Equal(t, 1, api.updates, "every change goes in one call")
	require.Equal(t, desired["Policy"], aws.ToString(api.lastUpdate.Policy))
	require.Nil(t, api.lastUpdate.Role)
	require.Equal(t, []int64{}, api.lastUpdate.PosixProfile.SecondaryGids, "removed secondary groups are sent as an empty list")
	properties, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, with(desired, map[string]any{
		"Role":              nil,
		"HomeDirectoryType": "PATH",
		"PosixProfile":      map[string]any{"Uid": float64(1000), "Gid": float64(100), "SecondaryGids": []any{}},
	}), properties)

	api.errs["UpdateAccess"] = fault("ResourceNotFoundException")
	require.ErrorIs(t, update(t, h, desired, with(desired, map[string]any{"Policy": "{}"})), native.ErrNotFound)
	var failure *native.Failure
	require.ErrorAs(t, update(t, h, desired, with(desired, map[string]any{"Policy": 2})), &failure)
}

func Test_DeleteAndProgress(t *testing.T) {
	h, api := newHandler()
	require.ErrorIs(t, h.Delete(t.Context(), region, identifier), native.ErrNotFound)
	_, err := h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: access})
	require.NoError(t, err)
	for _, op := range []native.Operation{native.OperationCreate, native.OperationUpdate} {
		progress, err := h.Progress(t.Context(), region, identifier, op)
		require.NoError(t, err)
		require.True(t, progress.Done, op)
	}
	progress, err := h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "exists", Waiting: "deleted"}, progress)

	api.errs["DeleteAccess"] = fault("ThrottlingException")
	err = h.Delete(t.Context(), region, identifier)
	require.ErrorContains(t, err, "ThrottlingException")
	require.NotErrorIs(t, err, native.ErrNotFound)
	delete(api.errs, "DeleteAccess")
	require.NoError(t, h.Delete(t.Context(), region, identifier))
	_, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
}

// Test_RedeployEmptyValues checks that a template that sets HomeDirectory, Policy, or SecondaryGids empty matches
// AWS, which reports an access without them, so a redeploy sends nothing.
func Test_RedeployEmptyValues(t *testing.T) {
	h, _ := newHandler()
	template := with(access, map[string]any{
		"HomeDirectory": "",
		"Policy":        "",
		"PosixProfile":  map[string]any{"Uid": float64(1000), "Gid": float64(100), "SecondaryGids": []any{}},
	})
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template})
	require.NoError(t, err)
	current, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Empty(t, patchOf(t, current, template, template))
	require.Empty(t, patchOf(t, current, nil, template), "a take-over sends nothing either")
}

// Test_UpdateKeepsValuesOutsideTemplate checks that an update sends what UCP's patch makes of AWS's values, so a
// nested value the template never set, such as secondary groups added outside Radius, stays as AWS has it.
func Test_UpdateKeepsValuesOutsideTemplate(t *testing.T) {
	template := with(access, map[string]any{
		"PosixProfile":          map[string]any{"Uid": float64(1000), "Gid": float64(100)},
		"HomeDirectoryType":     "LOGICAL",
		"HomeDirectoryMappings": []any{map[string]any{"Entry": "/", "Target": "/bucket/home"}},
		"HomeDirectory":         nil,
	})
	for name, lastApplied := range map[string]map[string]any{"after a deploy": template, "on a take-over": nil} {
		t.Run(name, func(t *testing.T) {
			h, api := newHandler()
			_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(template, map[string]any{
				"PosixProfile":          map[string]any{"Uid": float64(1000), "Gid": float64(100), "SecondaryGids": []any{float64(4), float64(5)}},
				"HomeDirectoryMappings": []any{map[string]any{"Entry": "/", "Target": "/bucket/home", "Type": "FILE"}},
			})})
			require.NoError(t, err)

			require.NoError(t, update(t, h, lastApplied, with(template, map[string]any{"PosixProfile": map[string]any{"Uid": float64(2000), "Gid": float64(100)}})))
			require.Equal(t, int64(2000), aws.ToInt64(api.lastUpdate.PosixProfile.Uid))
			require.Equal(t, []int64{4, 5}, api.lastUpdate.PosixProfile.SecondaryGids, "the secondary groups AWS has are kept")
			if lastApplied == nil {
				// UCP can't tell AWS's defaults from values set outside Radius, so it sends the template's list.
				require.Len(t, api.lastUpdate.HomeDirectoryMappings, 1)
				return
			}
			require.Nil(t, api.lastUpdate.HomeDirectoryMappings, "a list whose values the template sets as AWS has them isn't sent")
			properties, err := h.Read(t.Context(), region, identifier)
			require.NoError(t, err)
			require.Equal(t, "FILE", properties["HomeDirectoryMappings"].([]any)[0].(map[string]any)["Type"])
		})
	}
}

func Test_UpdatePatchDoesNotApply(t *testing.T) {
	h, api := newHandler()
	patch := jsondiff.Patch{{Type: jsondiff.OperationRemove, Path: "/Missing"}}
	var failure *native.Failure
	require.ErrorAs(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Current: access, Desired: access, Patch: patch}), &failure)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, "doesn't apply to the access AWS has")
	require.Zero(t, api.updates)
}

// Test_UpdateRole checks that a Role UCP's patch adds, because its fingerprint changed, is sent.
func Test_UpdateRole(t *testing.T) {
	h, api := newHandler()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: access})
	require.NoError(t, err)
	current, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	other := "arn:aws:iam::123456789012:role/other"
	patch := jsondiff.Patch{{Type: jsondiff.OperationAdd, Path: "/Role", Value: other}}
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: with(access, map[string]any{"Role": other}), Patch: patch}))
	require.Equal(t, other, aws.ToString(api.lastUpdate.Role))
	require.Nil(t, api.lastUpdate.HomeDirectory, "only Role is sent")
	require.Equal(t, other, api.store.Resource(accessType, identifier)["Role"])
}

// Test_ReadDefaults checks what Read reports for values DescribeAccess leaves out: no Role, PATH, DIRECTORY, and
// empty strings and lists, so a template that sets AWS's defaults redeploys with no change.
func Test_ReadDefaults(t *testing.T) {
	h, api := newHandler()
	api.store.Put(accessType, identifier, map[string]any{
		"ServerId": server, "ExternalId": externalID, "Role": role,
		"HomeDirectoryMappings": []any{map[string]any{"Entry": "/", "Target": "/bucket"}},
	})
	properties, err := h.Read(t.Context(), region, identifier)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"ServerId": server, "ExternalId": externalID, "HomeDirectory": "", "Policy": "", "HomeDirectoryType": "PATH",
		"HomeDirectoryMappings": []any{map[string]any{"Entry": "/", "Target": "/bucket", "Type": "DIRECTORY"}},
	}, properties)

	template := map[string]any{
		"ServerId": server, "ExternalId": externalID, "Role": role, "HomeDirectoryType": "PATH",
		"HomeDirectoryMappings": []any{map[string]any{"Entry": "/", "Target": "/bucket", "Type": "DIRECTORY"}},
	}
	require.Empty(t, patchOf(t, properties, template, template))
	require.Empty(t, patchOf(t, properties, nil, template), "a take-over sends nothing either")
}

func Test_samePolicy(t *testing.T) {
	policy := `{"Version":"2012-10-17","Statement":[]}`
	tests := []struct {
		path, template, aws string
		want                bool
	}{
		{"Policy", policy, `{ "Statement": [], "Version": "2012-10-17" }`, true},
		{"Policy", policy, `{"Version":"2012-10-17","Statement":[{}]}`, false},
		{"Policy", policy, "not json", false},
		{"Policy", "not json", "not json", false},
		{"HomeDirectory", policy, policy, false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, samePolicy(tt.path, tt.template, tt.aws), "%s %s %s", tt.path, tt.template, tt.aws)
	}
	require.False(t, samePolicy("Policy", map[string]any{}, policy))
}
