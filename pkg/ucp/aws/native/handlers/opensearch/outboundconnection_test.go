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
	"maps"
	"os"
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

const (
	connAPIReference = "https://docs.aws.amazon.com/opensearch-service/latest/APIReference/"
	connGuide        = "https://docs.aws.amazon.com/opensearch-service/latest/developerguide/cross-cluster-search.html"
)

// connSchemaFile returns a file of the type's hand-written schema.
func connSchemaFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// connDomainProperty returns a LocalDomainInfo or RemoteDomainInfo value for the domain, with owner and region when
// they aren't "".
func connDomainProperty(name, owner, region string) map[string]any {
	info := map[string]any{"DomainName": name}
	if owner != "" {
		info["OwnerId"] = owner
	}
	if region != "" {
		info["Region"] = region
	}
	return map[string]any{"AWSDomainInformation": info}
}

// connTemplate returns a template for a connection named archive from logs to archive, with changes.
func connTemplate(changes map[string]any) map[string]any {
	out := map[string]any{
		"ConnectionAlias":  "archive",
		"LocalDomainInfo":  connDomainProperty("logs", "", ""),
		"RemoteDomainInfo": connDomainProperty("archive", "", ""),
	}
	maps.Copy(out, changes)
	return out
}

// connCreateInput returns the input of a create of a connection named archive from logs to archive, with edits.
func connCreateInput(edits ...func(*opensearch.CreateOutboundConnectionInput)) *opensearch.CreateOutboundConnectionInput {
	in := &opensearch.CreateOutboundConnectionInput{
		ConnectionAlias:  aws.String("archive"),
		LocalDomainInfo:  &types.DomainInformationContainer{AWSDomainInformation: &types.AWSDomainInformation{DomainName: aws.String("logs")}},
		RemoteDomainInfo: &types.DomainInformationContainer{AWSDomainInformation: &types.AWSDomainInformation{DomainName: aws.String("archive")}},
	}
	for _, edit := range edits {
		edit(in)
	}
	return in
}

// connCall is a call against a fake API, after any calls it needs to set up.
type connCall func(ctx context.Context, f *connFake) error

// connAgainst returns calls as a rule's Break or Check, made in order against one fake API backed by store.
func connAgainst(calls ...connCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newConnFake(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

func connCreate(edits ...func(*opensearch.CreateOutboundConnectionInput)) connCall {
	return func(ctx context.Context, f *connFake) error {
		_, err := f.CreateOutboundConnection(ctx, connCreateInput(edits...))
		return err
	}
}

// connStatuses reads the connection until it reaches want, and fails when it passes through a status other than
// those in path first.
func connStatuses(id string, path ...types.OutboundConnectionStatusCode) connCall {
	return func(ctx context.Context, f *connFake) error {
		seen := []types.OutboundConnectionStatusCode{}
		for range 20 {
			c, err := connDescribe(ctx, f, id)
			if err != nil {
				return err
			}
			status := types.OutboundConnectionStatusCode("")
			if c != nil {
				status = c.ConnectionStatus.StatusCode
			}
			if len(seen) == 0 || seen[len(seen)-1] != status {
				seen = append(seen, status)
			}
			if status == path[len(path)-1] {
				break
			}
		}
		if !slices.Equal(seen, path) {
			return fmt.Errorf("the connection went through %v, not %v", seen, path)
		}
		return nil
	}
}

// connDescribe returns the connection from every page of DescribeOutboundConnections, or nil when it isn't listed.
func connDescribe(ctx context.Context, f *connFake, id string) (*types.OutboundConnection, error) {
	in := &opensearch.DescribeOutboundConnectionsInput{}
	var found *types.OutboundConnection
	for {
		page, err := f.DescribeOutboundConnections(ctx, in)
		if err != nil {
			return nil, err
		}
		for i := range page.Connections {
			if aws.ToString(page.Connections[i].ConnectionId) == id {
				found = &page.Connections[i]
			}
		}
		if in.NextToken = page.NextToken; in.NextToken == nil {
			return found, nil
		}
	}
}

func connDelete(id string) connCall {
	return func(ctx context.Context, f *connFake) error {
		_, err := f.DeleteOutboundConnection(ctx, &opensearch.DeleteOutboundConnectionInput{ConnectionId: aws.String(id)})
		return err
	}
}

func connAccept(id string) connCall {
	return func(ctx context.Context, f *connFake) error {
		f.accept(id)
		return nil
	}
}

const connFirstID = "cc-000000000001"

var connConstraintNote = " The page names no error for a value outside its length, pattern, or valid values, or for a required value left out; " +
	"the fake returns ValidationError, from the Common Errors page."

// connAPI is the record of the AWS calls the outbound connection handler makes.
var connAPI = map[string]nativetest.Call{
	"CreateOutboundConnection": {
		Doc: connAPIReference + "API_CreateOutboundConnection.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "ConnectionAlias is 2 to 100 characters, a letter followed by letters, digits, hyphens, and underscores",
				Break: connAgainst(connCreate(func(in *opensearch.CreateOutboundConnectionInput) { in.ConnectionAlias = aws.String("1archive") })),
				Code:  connValidationCode,
			},
			{
				Rule:  "ConnectionAlias, LocalDomainInfo, and RemoteDomainInfo are required",
				Break: connAgainst(connCreate(func(in *opensearch.CreateOutboundConnectionInput) { in.RemoteDomainInfo = nil })),
				Code:  connValidationCode,
			},
			{
				Rule: "DomainName is 3 to 28 characters matching [a-z][a-z0-9\\-]+, OwnerId is 12 digits, and Region is 5 to 30 characters",
				Doc:  connAPIReference + "API_AWSDomainInformation.html",
				Break: connAgainst(connCreate(func(in *opensearch.CreateOutboundConnectionInput) {
					in.RemoteDomainInfo.AWSDomainInformation.OwnerId = aws.String("12345")
				})),
				Code: connValidationCode,
			},
			{
				Rule:  "ConnectionMode is DIRECT or VPC_ENDPOINT",
				Break: connAgainst(connCreate(func(in *opensearch.CreateOutboundConnectionInput) { in.ConnectionMode = "PEERING" })),
				Code:  connValidationCode,
			},
			{
				Rule: "SkipUnavailable is ENABLED or DISABLED",
				Doc:  connAPIReference + "API_CrossClusterSearchConnectionProperties.html",
				Break: connAgainst(connCreate(func(in *opensearch.CreateOutboundConnectionInput) {
					in.ConnectionProperties = &types.ConnectionProperties{CrossClusterSearch: &types.CrossClusterSearchConnectionProperties{SkipUnavailable: "YES"}}
				})),
				Code: connValidationCode,
			},
			{
				Rule: "Endpoint matches ^[A-Za-z0-9\\-\\.]+$",
				Doc:  connAPIReference + "API_ConnectionProperties.html",
				Break: connAgainst(connCreate(func(in *opensearch.CreateOutboundConnectionInput) {
					in.ConnectionProperties = &types.ConnectionProperties{Endpoint: aws.String("https://archive/")}
				})),
				Code: connValidationCode,
			},
			{
				Rule: "A domain has at most 20 outgoing connections",
				Doc:  connGuide,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newConnFake(store)
					for i := range connMaxOutgoing + 1 {
						if _, err := f.CreateOutboundConnection(ctx, connCreateInput(func(in *opensearch.CreateOutboundConnectionInput) {
							in.ConnectionAlias = aws.String(fmt.Sprintf("archive%d", i))
						})); err != nil {
							return err
						}
					}
					return nil
				},
				Code: "LimitExceededException",
			},
			{
				Rule:  "AWS rejects a connection that already exists",
				Break: connAgainst(connCreate(), connCreate()),
				Code:  "ResourceAlreadyExistsException",
			},
			{
				Rule: "A new connection is VALIDATING, then PENDING_ACCEPTANCE until the destination domain's owner accepts it, then ACTIVE",
				Doc:  connAPIReference + "API_OutboundConnectionStatus.html",
				Check: connAgainst(
					connCreate(),
					connStatuses(connFirstID, "", types.OutboundConnectionStatusCodeValidating, types.OutboundConnectionStatusCodePendingAcceptance),
					connAccept(connFirstID),
					connStatuses(connFirstID, types.OutboundConnectionStatusCodeApproved, types.OutboundConnectionStatusCodeProvisioning, types.OutboundConnectionStatusCodeActive),
				),
			},
			{
				Rule: "Validation fails for a domain without fine-grained access control",
				Doc:  connGuide,
				Check: connAgainst(
					connCreate(func(in *opensearch.CreateOutboundConnectionInput) {
						in.RemoteDomainInfo.AWSDomainInformation.DomainName = aws.String("plain")
					}),
					connStatuses(connFirstID, "", types.OutboundConnectionStatusCodeValidating, types.OutboundConnectionStatusCodeValidationFailed),
				),
			},
			{
				Rule: "The source domain must be on the same or a higher version than the destination domain",
				Doc:  connGuide,
				Check: connAgainst(
					connCreate(func(in *opensearch.CreateOutboundConnectionInput) {
						in.LocalDomainInfo.AWSDomainInformation.DomainName = aws.String("old")
					}),
					connStatuses(connFirstID, "", types.OutboundConnectionStatusCodeValidating, types.OutboundConnectionStatusCodeValidationFailed),
				),
			},
			{
				Rule: "To connect domains across Regions, both must be on Elasticsearch 7.10 or later or OpenSearch",
				Doc:  connGuide,
				Check: connAgainst(
					connCreate(func(in *opensearch.CreateOutboundConnectionInput) {
						in.LocalDomainInfo.AWSDomainInformation.DomainName = aws.String("legacy")
						in.RemoteDomainInfo.AWSDomainInformation = &types.AWSDomainInformation{DomainName: aws.String("far-legacy"), Region: aws.String("us-east-1")}
					}),
					connStatuses(connFirstID, "", types.OutboundConnectionStatusCodeValidating, types.OutboundConnectionStatusCodeValidationFailed),
				),
			},
			{
				Rule: "An Elasticsearch domain can't connect to an OpenSearch domain",
				Doc:  connGuide,
				Check: connAgainst(
					connCreate(func(in *opensearch.CreateOutboundConnectionInput) {
						in.LocalDomainInfo.AWSDomainInformation.DomainName = aws.String("legacy")
					}),
					connStatuses(connFirstID, "", types.OutboundConnectionStatusCodeValidating, types.OutboundConnectionStatusCodeValidationFailed),
				),
			},
		},
		Unclear: "The page lists ResourceAlreadyExistsException but not which values make a duplicate, and the docs don't say whether a " +
			"DELETED, REJECTED, or VALIDATION_FAILED connection keeps its alias. The fake rejects a duplicate only when a live " +
			"connection has the same source domain, alias, and destination domain. The docs don't say what OwnerId and Region " +
			"default to; the fake uses the caller's account and Region. They don't say whether AWS lists a new connection at once, " +
			"or takes the call while the source domain is processing a change; the fake leaves a new connection out of the first " +
			"read, and rejects the call then with ConflictException." + connConstraintNote,
	},
	"DescribeOutboundConnections": {
		Doc: connAPIReference + "API_DescribeOutboundConnections.html",
		Rules: []nativetest.Rule{
			{
				Rule: "MaxResults is at most 100",
				Break: connAgainst(func(ctx context.Context, f *connFake) error {
					_, err := f.DescribeOutboundConnections(ctx, &opensearch.DescribeOutboundConnectionsInput{MaxResults: 101})
					return err
				}),
				Code: connValidationCode,
			},
			{
				Rule: "A NextToken AWS didn't return is rejected",
				Break: connAgainst(func(ctx context.Context, f *connFake) error {
					_, err := f.DescribeOutboundConnections(ctx, &opensearch.DescribeOutboundConnectionsInput{NextToken: aws.String("bogus")})
					return err
				}),
				Code: "InvalidPaginationTokenException",
			},
			{
				Rule: "A deleted connection is listed as DELETED for 15 days",
				Doc:  connGuide,
				Check: connAgainst(
					connCreate(),
					connStatuses(connFirstID, "", types.OutboundConnectionStatusCodeValidating, types.OutboundConnectionStatusCodePendingAcceptance),
					connDelete(connFirstID),
					connStatuses(connFirstID, types.OutboundConnectionStatusCodeDeleting, types.OutboundConnectionStatusCodeDeleted),
					connStatuses(connFirstID, types.OutboundConnectionStatusCodeDeleted),
				),
			},
		},
		Unclear: "The page doesn't name the filters it takes, so the handler sends none and reads every page; the fake rejects " +
			"filters. It doesn't give the page size." + connConstraintNote,
	},
	"DeleteOutboundConnection": {
		Doc: connAPIReference + "API_DeleteOutboundConnection.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "ConnectionId is 10 to 256 characters matching [a-z][a-z0-9\\-]+",
				Break: connAgainst(connDelete("cc-1")),
				Code:  connValidationCode,
			},
			{
				Rule:  "AWS rejects a delete of a connection that doesn't exist with ResourceNotFoundException",
				Break: connAgainst(connDelete("cc-000000000099")),
				Code:  "ResourceNotFoundException",
			},
		},
		Unclear: "The docs don't say whether AWS deletes a connection it is still validating, provisioning, rejecting, or " +
			"deleting, or one whose source domain is processing a change; the fake rejects the call then with ConflictException. " +
			"They don't say what a delete of a DELETED connection does; the fake answers ResourceNotFoundException." + connConstraintNote,
	},
}

func Test_Suite_OutboundConnection(t *testing.T) {
	properties := connTemplate(nil)
	next := 0
	nativetest.Run(t, nativetest.Suite{
		TypeName:    outboundConnectionType,
		Schema:      connSchemaFile(t, "aws-opensearchservice-outboundconnection.json"),
		Metadata:    connSchemaFile(t, "aws-opensearchservice-outboundconnection.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			f := newConnFake(store)
			// A DELETE of a record whose create response was lost finds the connection only with Find, which can't
			// see a connection AWS doesn't list yet. Test_OutboundConnection_ResendFindsLostCreate covers the lag.
			f.lagReads = 0
			return NewOutboundConnection(f)
		},
		IdentifierProperty: "ConnectionId",
		Properties:         properties,
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          properties,
		CreateOnlyChange: connTemplate(map[string]any{"ConnectionAlias": "archive2"}),
		CloudControlCreate: func(p map[string]any) (string, map[string]any) {
			next++
			id := fmt.Sprintf("cc-cloudcontrol-%d", next)
			out := maps.Clone(p)
			out["ConnectionId"] = id
			return id, out
		},
		Invalid: []map[string]any{
			connTemplate(map[string]any{"ConnectionAlias": "1archive"}),
			connTemplate(map[string]any{"LocalDomainInfo": connDomainProperty("Logs", "", "")}),
			connTemplate(map[string]any{"RemoteDomainInfo": connDomainProperty("archive", "12345", "")}),
			connTemplate(map[string]any{"RemoteDomainInfo": map[string]any{"DomainName": "archive"}}),
			connTemplate(map[string]any{"ConnectionMode": "PEERING"}),
			connTemplate(map[string]any{"ConnectionProperties": map[string]any{"Endpoint": "https://archive/"}}),
			connTemplate(map[string]any{"ConnectionProperties": map[string]any{"CrossClusterSearch": map[string]any{"SkipUnavailable": "YES"}}}),
		},
		API: connAPI,
	})
}

// newConnHandler returns a handler and its fake, backed by a new store.
func newConnHandler() (*OutboundConnection, *connFake) {
	f := newConnFake(awsfake.NewCloudControl())
	return NewOutboundConnection(f), f
}

func connRequest(properties map[string]any) native.CreateRequest {
	return native.CreateRequest{Region: connRegion, Properties: properties, ClientToken: "token-1"}
}

// connPut stores a connection the way AWS would have it, as if made outside Radius, in status.
func connPut(f *connFake, id string, properties map[string]any, status types.OutboundConnectionStatusCode) {
	stored := storedConnection(id, connCreateInput())
	maps.Copy(stored, properties)
	f.store.Put(outboundConnectionType, id, stored)
	f.status[id] = status
}

// requireConnRejected checks that err is a client failure AWS applied none of, and not a conflict.
func requireConnRejected(t *testing.T, err error, contains string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError, failure.Message)
	require.False(t, failure.Conflict, failure.Message)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, contains)
}

// requireConnConflict checks that err is a conflict AWS applied none of.
func requireConnConflict(t *testing.T, err error, contains string) {
	t.Helper()
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.Conflict, failure.Message)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Contains(t, failure.Message, contains)
}

// connWaitCreate reads the create's progress until it is done, and returns the progress and how many reads it took.
func connWaitCreate(t *testing.T, h *OutboundConnection, id string) (native.Progress, int) {
	t.Helper()
	for i := 1; i <= 20; i++ {
		p, err := h.Progress(t.Context(), connRegion, id, native.OperationCreate)
		require.NoError(t, err)
		if p.Done {
			return p, i
		}
	}
	t.Fatal("the create never finished")
	return native.Progress{}, 0
}

// Test_OutboundConnection_CreateWaitsForValidation checks that a create waits while AWS doesn't list the connection
// yet and while it is VALIDATING, and is done at PENDING_ACCEPTANCE, without waiting for the destination domain's owner.
func Test_OutboundConnection_CreateWaitsForValidation(t *testing.T) {
	h, f := newConnHandler()
	properties := connTemplate(map[string]any{
		"ConnectionMode":       "DIRECT",
		"ConnectionProperties": map[string]any{"CrossClusterSearch": map[string]any{"SkipUnavailable": "ENABLED"}},
		"RemoteDomainInfo":     connDomainProperty("shared", connOtherAccount, ""),
	})
	id, err := h.Create(t.Context(), connRequest(properties))
	require.NoError(t, err)

	first, err := h.Progress(t.Context(), connRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, first.Done, "AWS doesn't list the connection yet")
	require.Equal(t, "not listed", first.State)

	p, reads := connWaitCreate(t, h, id)
	require.Nil(t, p.Failure)
	require.Equal(t, "PENDING_ACCEPTANCE", p.State)
	require.Greater(t, reads, 1, "the connection is VALIDATING first")

	got, err := h.Read(t.Context(), connRegion, id)
	require.NoError(t, err)
	nativetest.RequireSubset(t, properties, got)
	require.Equal(t, map[string]any{"StatusCode": "PENDING_ACCEPTANCE"}, got["ConnectionStatus"])
	require.Equal(t, id, got["ConnectionId"])

	// The destination domain's owner accepts it later, outside Radius.
	f.accept(id)
	for range 10 {
		got, err = h.Read(t.Context(), connRegion, id)
		require.NoError(t, err)
	}
	require.Equal(t, map[string]any{"StatusCode": "ACTIVE"}, got["ConnectionStatus"])
}

// Test_OutboundConnection_CreateFails checks that a create fails at VALIDATION_FAILED and when the destination
// domain's owner rejects it, naming AWS's message, and that a delete then removes the connection.
func Test_OutboundConnection_CreateFails(t *testing.T) {
	t.Run("validation failed", func(t *testing.T) {
		h, _ := newConnHandler()
		id, err := h.Create(t.Context(), connRequest(connTemplate(map[string]any{"RemoteDomainInfo": connDomainProperty("plain", "", "")})))
		require.NoError(t, err)
		p, _ := connWaitCreate(t, h, id)
		require.NotNil(t, p.Failure)
		require.Equal(t, "VALIDATION_FAILED", p.State)
		require.Contains(t, p.Failure.Message, "Domains don't meet the requirements")

		require.NoError(t, h.Delete(t.Context(), connRegion, id))
		requireConnDeleted(t, h, id)
	})
	t.Run("rejected", func(t *testing.T) {
		h, f := newConnHandler()
		id, err := h.Create(t.Context(), connRequest(connTemplate(nil)))
		require.NoError(t, err)
		f.hold = true
		f.reject(id)
		f.hold = false
		p, _ := connWaitCreate(t, h, id)
		require.NotNil(t, p.Failure)
		require.Equal(t, "REJECTING", p.State)
	})
}

// requireConnDeleted waits for a delete, which reports DELETING first and is done once AWS reports DELETED.
func requireConnDeleted(t *testing.T, h *OutboundConnection, id string) {
	t.Helper()
	p, err := h.Progress(t.Context(), connRegion, id, native.OperationDelete)
	require.NoError(t, err)
	require.False(t, p.Done)
	require.Equal(t, "DELETING", p.State)
	for range 10 {
		if _, err = h.Progress(t.Context(), connRegion, id, native.OperationDelete); err != nil {
			break
		}
	}
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Read(t.Context(), connRegion, id)
	require.ErrorIs(t, err, native.ErrNotFound, "AWS lists the connection as DELETED")
	require.ErrorIs(t, h.Delete(t.Context(), connRegion, id), native.ErrNotFound)
	_, err = h.Progress(t.Context(), connRegion, id, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound, "a deleted connection is gone, not lagging")
}

// Test_OutboundConnection_ResendFindsLostCreate loses a create's response: the next deploy's Find misses the
// connection AWS doesn't list yet, so UCP sends the create again, which AWS rejects as a duplicate. Create returns a
// conflict, never the connection, because UCP doesn't check who manages an identifier Create returns. The deploy
// after that finds the connection with Find, which UCP checks.
func Test_OutboundConnection_ResendFindsLostCreate(t *testing.T) {
	for name, properties := range map[string]map[string]any{
		"same template": connTemplate(nil),
		"defaults set": connTemplate(map[string]any{
			"LocalDomainInfo":  connDomainProperty("logs", connAccount, connRegion),
			"RemoteDomainInfo": connDomainProperty("archive", connAccount, connRegion),
		}),
	} {
		t.Run(name, func(t *testing.T) {
			h, f := newConnHandler()
			_, err := f.CreateOutboundConnection(t.Context(), connCreateInput())
			require.NoError(t, err)
			request := connRequest(properties)

			found, err := h.Find(t.Context(), request)
			require.NoError(t, err)
			require.Empty(t, found, "AWS doesn't list the connection yet")

			id, err := h.Create(t.Context(), request)
			requireConnConflict(t, err, "deploy again to take it")
			require.Contains(t, err.Error(), connFirstID)
			require.Empty(t, id)
			require.Equal(t, 2, f.calls["CreateOutboundConnection"])
			require.Len(t, f.store.List(outboundConnectionType), 1)

			found, err = h.Find(t.Context(), request)
			require.NoError(t, err)
			require.Equal(t, connFirstID, found)
		})
	}
}

// Test_OutboundConnection_DuplicateNotListedYet checks that a create AWS rejects as a duplicate it doesn't list yet
// is a conflict, so the caller deploys again, and sends nothing more.
func Test_OutboundConnection_DuplicateNotListedYet(t *testing.T) {
	h, f := newConnHandler()
	f.lagReads = 3
	_, err := f.CreateOutboundConnection(t.Context(), connCreateInput())
	require.NoError(t, err)
	found, err := h.Find(t.Context(), connRequest(connTemplate(nil)))
	require.NoError(t, err)
	require.Empty(t, found)
	_, err = h.Create(t.Context(), connRequest(connTemplate(nil)))
	requireConnConflict(t, err, "doesn't list yet")
	require.Len(t, f.store.List(outboundConnectionType), 1)
}

// Test_OutboundConnection_DuplicateOfAnother checks that a create AWS rejects because the source domain has another
// connection with the alias is rejected, naming that connection, and isn't taken as the template's.
func Test_OutboundConnection_DuplicateOfAnother(t *testing.T) {
	tests := []struct {
		name   string
		stored map[string]any
		status types.OutboundConnectionStatusCode
	}{
		{name: "another destination", stored: map[string]any{"RemoteDomainInfo": connDomainProperty("metrics", connAccount, connRegion)}, status: types.OutboundConnectionStatusCodeActive},
		{name: "another owner of the destination", stored: map[string]any{"RemoteDomainInfo": connDomainProperty("archive", connOtherAccount, connRegion)}, status: types.OutboundConnectionStatusCodeActive},
		{name: "rejected", status: types.OutboundConnectionStatusCodeRejected},
		{name: "deleting", status: types.OutboundConnectionStatusCodeDeleting},
		{name: "validation failed", status: types.OutboundConnectionStatusCodeValidationFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f := newConnHandler()
			connPut(f, "cc-outside00001", tt.stored, tt.status)
			found, err := h.Find(t.Context(), connRequest(connTemplate(nil)))
			require.NoError(t, err)
			require.Empty(t, found)

			// AWS may count it as a duplicate, which the docs don't rule out.
			f.errs["CreateOutboundConnection"] = connFault("ResourceAlreadyExistsException", "already exists")
			_, err = h.Create(t.Context(), connRequest(connTemplate(nil)))
			requireConnRejected(t, err, "cc-outside00001 ("+string(tt.status))
		})
	}
}

// Test_OutboundConnection_Find checks which connections Find takes as the template's.
func Test_OutboundConnection_Find(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		h, f := newConnHandler()
		connPut(f, "cc-outside00001", nil, types.OutboundConnectionStatusCodePendingAcceptance)
		for _, properties := range []map[string]any{
			connTemplate(nil),
			connTemplate(map[string]any{
				"LocalDomainInfo":  connDomainProperty("logs", connAccount, connRegion),
				"RemoteDomainInfo": connDomainProperty("archive", connAccount, connRegion),
			}),
		} {
			found, err := h.Find(t.Context(), connRequest(properties))
			require.NoError(t, err)
			require.Equal(t, "cc-outside00001", found)
		}
		// A destination in another Region or account is another connection.
		for _, remote := range []map[string]any{connDomainProperty("archive", "", "us-east-1"), connDomainProperty("archive", connOtherAccount, "")} {
			found, err := h.Find(t.Context(), connRequest(connTemplate(map[string]any{"RemoteDomainInfo": remote})))
			require.NoError(t, err)
			require.Empty(t, found)
		}
	})
	t.Run("more than one", func(t *testing.T) {
		h, f := newConnHandler()
		connPut(f, "cc-outside00001", nil, types.OutboundConnectionStatusCodeActive)
		connPut(f, "cc-outside00002", nil, types.OutboundConnectionStatusCodeValidating)
		_, err := h.Find(t.Context(), connRequest(connTemplate(nil)))
		require.ErrorContains(t, err, "cc-outside00001")
		require.ErrorContains(t, err, "cc-outside00002")
	})
	t.Run("other settings", func(t *testing.T) {
		for name, properties := range map[string]map[string]any{
			"ConnectionMode": connTemplate(map[string]any{"ConnectionMode": "VPC_ENDPOINT"}),
			"SkipUnavailable": connTemplate(map[string]any{
				"ConnectionProperties": map[string]any{"CrossClusterSearch": map[string]any{"SkipUnavailable": "ENABLED"}},
			}),
			"Endpoint": connTemplate(map[string]any{"ConnectionProperties": map[string]any{"Endpoint": "archive.example.com"}}),
		} {
			t.Run(name, func(t *testing.T) {
				h, f := newConnHandler()
				connPut(f, "cc-outside00001", nil, types.OutboundConnectionStatusCodeActive)
				_, err := h.Find(t.Context(), connRequest(properties))
				requireConnRejected(t, err, name)

				f.errs["CreateOutboundConnection"] = connFault("ResourceAlreadyExistsException", "already exists")
				_, err = h.Create(t.Context(), connRequest(properties))
				requireConnRejected(t, err, name)
			})
		}
	})
	t.Run("matching settings", func(t *testing.T) {
		h, f := newConnHandler()
		connPut(f, "cc-outside00001", nil, types.OutboundConnectionStatusCodeActive)
		found, err := h.Find(t.Context(), connRequest(connTemplate(map[string]any{
			"ConnectionMode":       "DIRECT",
			"ConnectionProperties": map[string]any{"CrossClusterSearch": map[string]any{"SkipUnavailable": "DISABLED"}},
		})))
		require.NoError(t, err)
		require.Equal(t, "cc-outside00001", found)
	})
	t.Run("on page two", func(t *testing.T) {
		h, f := newConnHandler()
		for i := range 4 {
			connPut(f, fmt.Sprintf("cc-outside0000%d", i), map[string]any{"ConnectionAlias": fmt.Sprintf("other%d", i)}, types.OutboundConnectionStatusCodeActive)
		}
		connPut(f, "cc-outside00009", nil, types.OutboundConnectionStatusCodeActive)
		found, err := h.Find(t.Context(), connRequest(connTemplate(nil)))
		require.NoError(t, err)
		require.Equal(t, "cc-outside00009", found)
		require.Greater(t, f.calls["DescribeOutboundConnections"], 2)
	})
	t.Run("deleted", func(t *testing.T) {
		h, f := newConnHandler()
		f.deleted["cc-outside00001"] = storedConnection("cc-outside00001", connCreateInput())
		found, err := h.Find(t.Context(), connRequest(connTemplate(nil)))
		require.NoError(t, err)
		require.Empty(t, found)
		_, others, err := h.search(t.Context(), connRegion, connectionTemplate{alias: "archive", local: connectionDomain{name: "logs"}})
		require.NoError(t, err)
		require.Empty(t, others, "a DELETED connection holds nothing")
	})
	t.Run("list fails", func(t *testing.T) {
		h, f := newConnHandler()
		f.errs["DescribeOutboundConnections"] = errors.New("connection reset")
		_, err := h.Find(t.Context(), connRequest(connTemplate(nil)))
		require.ErrorContains(t, err, "connection reset")
	})
}

// Test_OutboundConnection_Invalid checks the templates and identifiers the handler rejects before any AWS call.
func Test_OutboundConnection_Invalid(t *testing.T) {
	h, f := newConnHandler()
	for name, properties := range map[string]map[string]any{
		"short alias":              connTemplate(map[string]any{"ConnectionAlias": "a"}),
		"alias not a string":       connTemplate(map[string]any{"ConnectionAlias": 5}),
		"unknown domain field":     connTemplate(map[string]any{"LocalDomainInfo": map[string]any{"AWSDomainInformation": map[string]any{"DomainName": "logs", "Arn": "x"}}}),
		"bad region":               connTemplate(map[string]any{"RemoteDomainInfo": connDomainProperty("archive", "", "US")}),
		"unknown property":         connTemplate(map[string]any{"ConnectionProperties": map[string]any{"Skip": "ENABLED"}}),
		"bad identifier":           {"ConnectionId": "CC-1"},
		"properties not an object": connTemplate(map[string]any{"ConnectionProperties": "x"}),
		"domain name not a string": connTemplate(map[string]any{"LocalDomainInfo": map[string]any{"AWSDomainInformation": map[string]any{"DomainName": 5}}}),
		"mode not a string":        connTemplate(map[string]any{"ConnectionMode": true}),
		"endpoint not a string":    connTemplate(map[string]any{"ConnectionProperties": map[string]any{"Endpoint": 5}}),
		"search not an object":     connTemplate(map[string]any{"ConnectionProperties": map[string]any{"CrossClusterSearch": "ENABLED"}}),
		"skip not a string":        connTemplate(map[string]any{"ConnectionProperties": map[string]any{"CrossClusterSearch": map[string]any{"SkipUnavailable": 1}}}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.Identifier(properties)
			requireConnRejected(t, err, "")
		})
	}
	id, err := h.Identifier(map[string]any{"ConnectionId": connFirstID})
	require.NoError(t, err)
	require.Equal(t, connFirstID, id)

	for name, properties := range map[string]map[string]any{
		"invalid alias":          connTemplate(map[string]any{"ConnectionAlias": "a"}),
		"missing alias":          {"LocalDomainInfo": connDomainProperty("logs", "", ""), "RemoteDomainInfo": connDomainProperty("archive", "", "")},
		"source in other Region": connTemplate(map[string]any{"LocalDomainInfo": connDomainProperty("logs", "", "us-east-1")}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.Create(t.Context(), connRequest(properties))
			requireConnRejected(t, err, "")
			_, err = h.Find(t.Context(), connRequest(properties))
			requireConnRejected(t, err, "")
		})
	}
	for _, op := range []func() error{
		func() error { _, err := h.Read(t.Context(), connRegion, "../cc-000000000001"); return err },
		func() error { return h.Delete(t.Context(), connRegion, "../cc-000000000001") },
	} {
		requireConnRejected(t, op(), "not the ID")
	}
	require.Empty(t, f.calls)
}

// Test_OutboundConnection_Busy checks that a create or delete AWS can't make now, because the source domain is
// processing a change or the connection is still moving on its own, is a conflict.
func Test_OutboundConnection_Busy(t *testing.T) {
	t.Run("create while the domain is processing", func(t *testing.T) {
		h, f := newConnHandler()
		f.processing[connDomainKey(connAccount, connRegion, "logs")] = 5
		_, err := h.Create(t.Context(), connRequest(connTemplate(nil)))
		requireConnConflict(t, err, "dependsOn")
		require.Empty(t, f.store.List(outboundConnectionType))
	})
	t.Run("delete while the domain is processing", func(t *testing.T) {
		h, f := newConnHandler()
		connPut(f, "cc-outside00001", nil, types.OutboundConnectionStatusCodeActive)
		f.processing[connDomainKey(connAccount, connRegion, "logs")] = 5
		requireConnConflict(t, h.Delete(t.Context(), connRegion, "cc-outside00001"), "cc-outside00001")
		require.Equal(t, types.OutboundConnectionStatusCodeActive, f.statusOf("cc-outside00001"))
	})
	for _, status := range []types.OutboundConnectionStatusCode{
		types.OutboundConnectionStatusCodeValidating, types.OutboundConnectionStatusCodeApproved,
		types.OutboundConnectionStatusCodeProvisioning, types.OutboundConnectionStatusCodeRejecting,
	} {
		t.Run("delete while "+string(status), func(t *testing.T) {
			h, f := newConnHandler()
			f.hold = true
			connPut(f, "cc-outside00001", nil, status)
			requireConnConflict(t, h.Delete(t.Context(), connRegion, "cc-outside00001"), string(status))
			require.Zero(t, f.calls["DeleteOutboundConnection"])
		})
	}
}

// Test_OutboundConnection_Delete checks the statuses a delete starts from.
func Test_OutboundConnection_Delete(t *testing.T) {
	for _, status := range []types.OutboundConnectionStatusCode{
		types.OutboundConnectionStatusCodePendingAcceptance, types.OutboundConnectionStatusCodeActive,
		types.OutboundConnectionStatusCodeRejected, types.OutboundConnectionStatusCodeValidationFailed,
	} {
		t.Run(string(status), func(t *testing.T) {
			h, f := newConnHandler()
			connPut(f, "cc-outside00001", nil, status)
			require.NoError(t, h.Delete(t.Context(), connRegion, "cc-outside00001"))
			requireConnDeleted(t, h, "cc-outside00001")
		})
	}
	t.Run("already deleting", func(t *testing.T) {
		h, f := newConnHandler()
		f.hold = true
		connPut(f, "cc-outside00001", nil, types.OutboundConnectionStatusCodeDeleting)
		require.NoError(t, h.Delete(t.Context(), connRegion, "cc-outside00001"))
		require.Zero(t, f.calls["DeleteOutboundConnection"])
	})
	t.Run("gone", func(t *testing.T) {
		h, f := newConnHandler()
		require.ErrorIs(t, h.Delete(t.Context(), connRegion, connFirstID), native.ErrNotFound)
		require.Zero(t, f.calls["DeleteOutboundConnection"])
	})
	t.Run("gone between the read and the delete", func(t *testing.T) {
		h, f := newConnHandler()
		connPut(f, "cc-outside00001", nil, types.OutboundConnectionStatusCodeActive)
		f.errs["DeleteOutboundConnection"] = connFault("ResourceNotFoundException", "not found")
		require.ErrorIs(t, h.Delete(t.Context(), connRegion, "cc-outside00001"), native.ErrNotFound)
	})
	t.Run("read fails", func(t *testing.T) {
		h, f := newConnHandler()
		f.errs["DescribeOutboundConnections"] = connFault("AccessDeniedException", "denied")
		requireConnRejected(t, h.Delete(t.Context(), connRegion, connFirstID), "denied")
		require.Zero(t, f.calls["DeleteOutboundConnection"])
	})
	t.Run("other error", func(t *testing.T) {
		h, f := newConnHandler()
		connPut(f, "cc-outside00001", nil, types.OutboundConnectionStatusCodeActive)
		f.errs["DeleteOutboundConnection"] = errors.New("connection reset")
		require.ErrorContains(t, h.Delete(t.Context(), connRegion, "cc-outside00001"), "connection reset")
	})
}

// Test_OutboundConnection_CreateErrors checks how Create reports AWS's other errors.
func Test_OutboundConnection_CreateErrors(t *testing.T) {
	h, f := newConnHandler()
	f.errs["CreateOutboundConnection"] = errors.New("connection reset")
	_, err := h.Create(t.Context(), connRequest(connTemplate(nil)))
	require.ErrorContains(t, err, "connection reset")
	var failure *native.Failure
	require.False(t, errors.As(err, &failure), "AWS may have made the connection")

	f.errs["CreateOutboundConnection"] = connFault("ResourceAlreadyExistsException", "already exists")
	f.errs["DescribeOutboundConnections"] = errors.New("connection reset")
	_, err = h.Create(t.Context(), connRequest(connTemplate(nil)))
	require.ErrorAs(t, err, &failure, "AWS made nothing")
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
}

// Test_OutboundConnection_List checks that List leaves out deleting and deleted connections, and fails when a page
// fails rather than leave connections out.
func Test_OutboundConnection_List(t *testing.T) {
	h, f := newConnHandler()
	f.hold = true
	connPut(f, "cc-outside00001", nil, types.OutboundConnectionStatusCodeActive)
	connPut(f, "cc-outside00002", map[string]any{"ConnectionAlias": "b"}, types.OutboundConnectionStatusCodeDeleting)
	connPut(f, "cc-outside00003", map[string]any{"ConnectionAlias": "c"}, types.OutboundConnectionStatusCodePendingAcceptance)
	f.deleted["cc-outside00004"] = storedConnection("cc-outside00004", connCreateInput())
	list, err := h.List(t.Context(), connRegion)
	require.NoError(t, err)
	ids := []string{}
	for _, r := range list {
		ids = append(ids, r.Identifier)
		require.Equal(t, r.Identifier, r.Properties["ConnectionId"])
	}
	require.ElementsMatch(t, []string{"cc-outside00001", "cc-outside00003"}, ids)

	f.errs["DescribeOutboundConnections"] = errors.New("connection reset")
	_, err = h.List(t.Context(), connRegion)
	require.ErrorContains(t, err, "connection reset")
}

// Test_OutboundConnection_Update checks that an update changes nothing in AWS.
func Test_OutboundConnection_Update(t *testing.T) {
	h, f := newConnHandler()
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: connRegion, Identifier: connFirstID}))
	p, err := h.Progress(t.Context(), connRegion, connFirstID, native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, p.Done)
	require.Empty(t, f.calls)
}

// connStub is an OpenSearch Service API that answers every call with what it holds, for answers the fake never gives.
type connStub struct {
	create      *opensearch.CreateOutboundConnectionOutput
	connections []types.OutboundConnection
	err         error
}

func (s *connStub) CreateOutboundConnection(ctx context.Context, in *opensearch.CreateOutboundConnectionInput, _ ...func(*opensearch.Options)) (*opensearch.CreateOutboundConnectionOutput, error) {
	return s.create, nil
}

func (s *connStub) DescribeOutboundConnections(ctx context.Context, in *opensearch.DescribeOutboundConnectionsInput, _ ...func(*opensearch.Options)) (*opensearch.DescribeOutboundConnectionsOutput, error) {
	return &opensearch.DescribeOutboundConnectionsOutput{Connections: s.connections}, s.err
}

func (s *connStub) DeleteOutboundConnection(ctx context.Context, in *opensearch.DeleteOutboundConnectionInput, _ ...func(*opensearch.Options)) (*opensearch.DeleteOutboundConnectionOutput, error) {
	return &opensearch.DeleteOutboundConnectionOutput{}, nil
}

// Test_OutboundConnection_OddAnswers checks answers AWS's docs allow but the fake never gives: a create with no ID,
// and a connection without a status or domains, which never matches a template and is never done.
func Test_OutboundConnection_OddAnswers(t *testing.T) {
	h := NewOutboundConnection(&connStub{create: &opensearch.CreateOutboundConnectionOutput{}})
	_, err := h.Create(t.Context(), connRequest(connTemplate(nil)))
	require.ErrorContains(t, err, "returned no ID")

	h = NewOutboundConnection(&connStub{connections: []types.OutboundConnection{{ConnectionId: aws.String(connFirstID), ConnectionAlias: aws.String("archive")}}})
	found, err := h.Find(t.Context(), connRequest(connTemplate(nil)))
	require.NoError(t, err)
	require.Empty(t, found)
	p, err := h.Progress(t.Context(), connRegion, connFirstID, native.OperationCreate)
	require.NoError(t, err)
	require.False(t, p.Done)

	h = NewOutboundConnection(&connStub{err: connFault("AccessDeniedException", "denied")})
	_, err = h.Progress(t.Context(), connRegion, connFirstID, native.OperationCreate)
	require.ErrorContains(t, err, "denied")
	require.False(t, errors.Is(err, native.ErrNotFound))
}
