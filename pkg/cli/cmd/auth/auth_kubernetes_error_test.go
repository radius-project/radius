// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInspectionCommandsPreserveKubernetesForbidden(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation string
		scope     string
		principal string
		path      string
		method    string
	}{
		{name: "whoami", operation: "whoami", path: "/providers/Radius.Core/whoAmI", method: http.MethodGet},
		{name: "status", operation: "status", path: "/providers/Radius.Core/whoAmI", method: http.MethodGet},
		{name: "installation access", operation: "check", scope: "/", path: "/providers/Radius.Core/checkAccess", method: http.MethodPost},
		{name: "scoped access", operation: "check", scope: "/planes/radius/local", path: "/planes/radius/local/providers/Radius.Core/checkAccess", method: http.MethodPost},
		{name: "principal issuer lookup", operation: "check", scope: "/", principal: "group:developers", path: "/providers/Radius.Core/whoAmI", method: http.MethodGet},
	} {
		for _, format := range []string{"json", "table"} {
			t.Run(test.name+"/"+format, func(t *testing.T) {
				message := `User "unbound-caller" cannot ` + test.method + ` path "` + test.path + `": forbidden`
				server, requests := inspectionHTTPServer(t, http.StatusForbidden, metav1.Status{
					TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
					Status:   metav1.StatusFailure, Reason: metav1.StatusReasonForbidden, Code: http.StatusForbidden, Message: message,
				})
				config := radcli.LoadConfigWithWorkspace(t)
				section, err := cli.ReadWorkspaceSection(config)
				require.NoError(t, err)
				workspace := section.Items[radcli.TestWorkspaceName]
				workspace.Connection["overrides"] = map[string]any{"ucp": server.URL}
				section.Items[radcli.TestWorkspaceName] = workspace
				cli.UpdateWorkspaceSection(config, section)
				var buffer bytes.Buffer
				cmd, _ := newTestCommand(&framework.Impl{
					ConfigHolder: &framework.ConfigHolder{Config: config},
					Output:       &output.OutputWriter{Writer: &buffer},
				}, test.operation)
				cmd.SilenceErrors, cmd.SilenceUsage = true, true
				args := []string{"--output", format}
				if test.operation == "check" {
					args = append(args, "--scope", test.scope, "--action", "Radius.Core/applications/read")
				}
				if test.principal != "" {
					args = append(args, "--principal", test.principal)
				}
				cmd.SetArgs(args)
				err = cmd.ExecuteContext(t.Context())
				require.EqualError(t, err, message)
				require.True(t, apierrors.IsForbidden(err))
				var statusError *apierrors.StatusError
				require.ErrorAs(t, err, &statusError)
				require.Equal(t, int32(http.StatusForbidden), statusError.Status().Code)
				require.Nil(t, clientv2.TryUnfoldResponseError(err), "root error formatting must not replace Kubernetes diagnostics with an empty ARM error")
				require.Empty(t, buffer.String(), "failed inspection must not emit a success result")
				requireInspectionRequest(t, requests, test.method, test.path)
				require.Empty(t, requests, "forbidden inspection must stop without retrying or checking access after a failed issuer lookup")
			})
		}
	}
}

type failedResponseReader struct{}

func (failedResponseReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failedResponseReader) Close() error             { return nil }

func TestInspectionErrorPreservesOtherFailures(t *testing.T) {
	t.Parallel()
	networkError := errors.New("connection interrupted")
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "network failure", err: networkError},
		{name: "response without raw HTTP", err: &azcore.ResponseError{StatusCode: http.StatusForbidden}},
		{name: "unreadable response", err: &azcore.ResponseError{StatusCode: http.StatusForbidden, RawResponse: &http.Response{Body: failedResponseReader{}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.err, inspectionError(test.err))
		})
	}
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "ARM authorization failure", body: `{"error":{"code":"AuthorizationFailed","message":"permission denied"}}`},
		{name: "invalid JSON", body: `{"kind":`},
		{name: "non-Status object", body: `{"kind":"Other","status":"Failure","message":"unexpected"}`},
		{name: "non-failure Status", body: `{"kind":"Status","status":"Success","message":"unexpected"}`},
		{name: "Status without diagnostic", body: `{"kind":"Status","status":"Failure","reason":"Forbidden"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := &http.Response{Body: io.NopCloser(bytes.NewBufferString(test.body))}
			err := &azcore.ResponseError{StatusCode: http.StatusForbidden, RawResponse: response}
			require.Same(t, err, inspectionError(err))
			body, readErr := io.ReadAll(response.Body)
			require.NoError(t, readErr)
			require.Equal(t, test.body, string(body), "error inspection must leave the original HTTP body available")
		})
	}
}
