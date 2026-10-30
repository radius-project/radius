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

package awsfake

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/require"
)

const vpc = "AWS::EC2::VPC"

func status(t *testing.T, c *CloudControl, token *string) *cctypes.ProgressEvent {
	t.Helper()
	out, err := c.GetResourceRequestStatus(t.Context(), &cloudcontrol.GetResourceRequestStatusInput{RequestToken: token})
	require.NoError(t, err)
	return out.ProgressEvent
}

func properties(t *testing.T, out *cloudcontrol.GetResourceOutput) map[string]any {
	t.Helper()
	got := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(aws.ToString(out.ResourceDescription.Properties)), &got))
	return got
}

func Test_CreateReadUpdateDelete(t *testing.T) {
	c := NewCloudControl()
	ctx := t.Context()

	_, err := c.GetResource(ctx, &cloudcontrol.GetResourceInput{TypeName: aws.String(vpc), Identifier: aws.String("id-1")})
	var notFound *cctypes.ResourceNotFoundException
	require.ErrorAs(t, err, &notFound)

	created, err := c.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     aws.String(vpc),
		DesiredState: aws.String(`{"CidrBlock":"10.0.0.0/16","Tags":[{"Key":"a"}],"Nested":{"A":"1"}}`),
		ClientToken:  aws.String("token"),
	})
	require.NoError(t, err)
	event := status(t, c, created.ProgressEvent.RequestToken)
	require.Equal(t, cctypes.OperationStatusSuccess, event.OperationStatus)
	require.Equal(t, cctypes.OperationCreate, event.Operation)
	require.Equal(t, "id-1", aws.ToString(event.Identifier))

	// The same client token returns the first request and creates nothing.
	again, err := c.CreateResource(ctx, &cloudcontrol.CreateResourceInput{TypeName: aws.String(vpc), DesiredState: aws.String(`{}`), ClientToken: aws.String("token")})
	require.NoError(t, err)
	require.Equal(t, aws.ToString(created.ProgressEvent.RequestToken), aws.ToString(again.ProgressEvent.RequestToken))
	require.Equal(t, 1, c.Count(vpc))

	patch := `[{"op":"replace","path":"/CidrBlock","value":"10.1.0.0/16"},{"op":"add","path":"/New/Inner","value":"x"},{"op":"remove","path":"/Nested/A"},{"op":"replace","path":"/Tags/0","value":{"Key":"b"}}]`
	_, err = c.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{TypeName: aws.String(vpc), Identifier: aws.String("id-1"), PatchDocument: aws.String(patch)})
	require.NoError(t, err)
	got, err := c.GetResource(ctx, &cloudcontrol.GetResourceInput{TypeName: aws.String(vpc), Identifier: aws.String("id-1")})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"CidrBlock": "10.1.0.0/16", "Tags": []any{map[string]any{"Key": "b"}}, "Nested": map[string]any{}, "New": map[string]any{"Inner": "x"}}, properties(t, got))

	calls := c.Calls()
	require.Equal(t, []string{"GetResource", "CreateResource", "CreateResource", "UpdateResource", "GetResource"}, func() []string {
		out := []string{}
		for _, call := range calls {
			out = append(out, call.Operation)
		}
		return out
	}())
	require.Equal(t, "token", calls[1].ClientToken)
	require.Empty(t, c.Calls())

	deleted, err := c.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{TypeName: aws.String(vpc), Identifier: aws.String("id-1")})
	require.NoError(t, err)
	require.Equal(t, cctypes.OperationDelete, status(t, c, deleted.ProgressEvent.RequestToken).Operation)
	require.Zero(t, c.Count(vpc))
	_, err = c.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{TypeName: aws.String(vpc), Identifier: aws.String("id-1")})
	require.ErrorAs(t, err, &notFound)
	_, err = c.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{TypeName: aws.String(vpc), Identifier: aws.String("id-1"), PatchDocument: aws.String("[]")})
	require.ErrorAs(t, err, &notFound)
}

func Test_IdentifiersAndWriteOnly(t *testing.T) {
	c := NewCloudControl()
	c.Identifiers["AWS::S3::Bucket"] = "BucketName"
	c.WriteOnly["AWS::S3::Bucket"] = []string{"Password", "Auth/Token", "Users/*/Secret"}
	_, err := c.CreateResource(t.Context(), &cloudcontrol.CreateResourceInput{
		TypeName:     aws.String("AWS::S3::Bucket"),
		DesiredState: aws.String(`{"BucketName":"assets","Password":"p","Auth":{"Token":"t","Mode":"m"},"Users":[{"Name":"u","Secret":"s"}]}`),
	})
	require.NoError(t, err)
	require.Equal(t, "p", c.Resource("AWS::S3::Bucket", "assets")["Password"], "the fake keeps write-only values")

	got, err := c.GetResource(t.Context(), &cloudcontrol.GetResourceInput{TypeName: aws.String("AWS::S3::Bucket"), Identifier: aws.String("assets")})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"BucketName": "assets", "Auth": map[string]any{"Mode": "m"}, "Users": []any{map[string]any{"Name": "u"}}}, properties(t, got))

	c.Put(vpc, "vpc-1", map[string]any{"VpcId": "vpc-1"})
	require.Equal(t, map[string]any{"VpcId": "vpc-1"}, c.Resource(vpc, "vpc-1"))
	c.Remove(vpc, "vpc-1")
	require.Nil(t, c.Resource(vpc, "vpc-1"))
}

func Test_FailuresHooksAndErrors(t *testing.T) {
	c := NewCloudControl()
	ctx := t.Context()

	c.FailNext = cctypes.HandlerErrorCodeInvalidRequest
	failed, err := c.CreateResource(ctx, &cloudcontrol.CreateResourceInput{TypeName: aws.String(vpc), DesiredState: aws.String(`{}`)})
	require.NoError(t, err)
	event := status(t, c, failed.ProgressEvent.RequestToken)
	require.Equal(t, cctypes.OperationStatusFailed, event.OperationStatus)
	require.Equal(t, cctypes.HandlerErrorCodeInvalidRequest, event.ErrorCode)
	require.Zero(t, c.Count(vpc), "a failed create changes nothing")

	c.PendingReads = 1
	created, err := c.CreateResource(ctx, &cloudcontrol.CreateResourceInput{TypeName: aws.String(vpc), DesiredState: aws.String(`{}`)})
	require.NoError(t, err)
	require.Equal(t, cctypes.OperationStatusInProgress, status(t, c, created.ProgressEvent.RequestToken).OperationStatus)
	require.Equal(t, cctypes.OperationStatusSuccess, status(t, c, created.ProgressEvent.RequestToken).OperationStatus)

	ran := 0
	c.Before["DeleteResource"] = func() { ran++ }
	_, _ = c.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{TypeName: aws.String(vpc), Identifier: aws.String("nope")})
	_, _ = c.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{TypeName: aws.String(vpc), Identifier: aws.String("nope")})
	require.Equal(t, 1, ran, "a hook runs once")

	_, err = c.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{RequestToken: aws.String("unknown")})
	var unknown *cctypes.RequestTokenNotFoundException
	require.ErrorAs(t, err, &unknown)

	boom := errors.New("boom")
	for _, op := range []string{"GetResource", "CreateResource", "UpdateResource", "DeleteResource", "GetResourceRequestStatus"} {
		c.Errors[op] = boom
	}
	_, err = c.GetResource(ctx, &cloudcontrol.GetResourceInput{})
	require.ErrorIs(t, err, boom)
	_, err = c.CreateResource(ctx, &cloudcontrol.CreateResourceInput{})
	require.ErrorIs(t, err, boom)
	_, err = c.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{})
	require.ErrorIs(t, err, boom)
	_, err = c.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{})
	require.ErrorIs(t, err, boom)
	_, err = c.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{})
	require.ErrorIs(t, err, boom)

	_, err = c.ListResources(ctx, nil)
	require.Error(t, err)
	_, err = c.CancelResourceRequest(ctx, nil)
	require.Error(t, err)
	_, err = c.ListResourceRequests(ctx, nil)
	require.Error(t, err)
}

func Test_InvalidDocuments(t *testing.T) {
	c := NewCloudControl()
	ctx := t.Context()
	_, err := c.CreateResource(ctx, &cloudcontrol.CreateResourceInput{TypeName: aws.String(vpc), DesiredState: aws.String("not json")})
	require.Error(t, err)

	c.Put(vpc, "id", map[string]any{"List": []any{map[string]any{"Name": "a"}}})
	for _, patch := range []string{
		"not json",
		`[{"op":"replace","path":"/List/5","value":"x"}]`,
		`[{"op":"replace","path":"/List/x/y","value":"x"}]`,
		`[{"op":"replace","path":"/List/5/Name","value":"x"}]`,
		`[{"op":"remove","path":"/List/0"}]`,
	} {
		_, err = c.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{TypeName: aws.String(vpc), Identifier: aws.String("id"), PatchDocument: aws.String(patch)})
		require.Error(t, err, patch)
	}
}

func Test_UpdateInsideArrayItem(t *testing.T) {
	c := NewCloudControl()
	c.Put(vpc, "id", map[string]any{"List": []any{map[string]any{"Name": "a"}}})
	_, err := c.UpdateResource(t.Context(), &cloudcontrol.UpdateResourceInput{
		TypeName: aws.String(vpc), Identifier: aws.String("id"),
		PatchDocument: aws.String(`[{"op":"replace","path":"/List/0/Name","value":"b"}]`),
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"List": []any{map[string]any{"Name": "b"}}}, c.Resource(vpc, "id"))
}

func Test_CloudFormation(t *testing.T) {
	cf := &CloudFormation{Schemas: map[string]string{vpc: `{}`}}
	out, err := cf.DescribeType(t.Context(), &cloudformation.DescribeTypeInput{TypeName: aws.String(vpc)})
	require.NoError(t, err)
	require.Equal(t, `{}`, aws.ToString(out.Schema))
	_, err = cf.DescribeType(t.Context(), &cloudformation.DescribeTypeInput{TypeName: aws.String("AWS::Missing::Type")})
	require.Error(t, err)
}
