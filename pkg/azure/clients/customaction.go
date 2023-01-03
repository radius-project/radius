// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
)

type CustomActionClient struct {
	resources.BaseClient
}

type CustomActionResponse struct {
	Body     map[string]any
	Response autorest.Response
}

func (client CustomActionClient) InvokeCustomAction(ctx context.Context, id string, apiVersion string, action string, body any) (result CustomActionResponse, err error) {
	req, err := client.InvokeCustomActionPreparer(ctx, id, apiVersion, action, body)
	if err != nil {
		err = autorest.NewErrorWithError(err, "CustomActionClient", "InvokeCustomAction", nil, "Failure preparing request")
		return
	}

	response, err := client.InvokeCustomActionSender(req)
	if err != nil {
		err = autorest.NewErrorWithError(err, "CustomActionClient", "InvokeCustomAction", nil, "Failure sending request")
		return
	}

	result, err = client.InvokeCustomActionResponder(response)
	if err != nil {
		err = autorest.NewErrorWithError(err, "CustomActionClient", "InvokeCustomAction", nil, "Failure reading response")
		return
	}

	return
}

func (client CustomActionClient) InvokeCustomActionPreparer(ctx context.Context, id string, apiVersion string, action string, body any) (*http.Request, error) {
	pathParameters := map[string]any{
		"id":     autorest.Encode("none", id),
		"action": autorest.Encode("path", action),
	}

	queryParameters := map[string]any{
		"api-version": apiVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPost(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPathParameters("{id}/{action}", pathParameters),
		autorest.WithQueryParameters(queryParameters))
	if body != nil {
		preparer = autorest.DecoratePreparer(preparer, autorest.WithJSON(body))
	}

	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

func (client CustomActionClient) InvokeCustomActionSender(req *http.Request) (*http.Response, error) {
	return client.Send(req, azure.DoRetryWithRegistration(client.Client))
}

func (client CustomActionClient) InvokeCustomActionResponder(resp *http.Response) (result CustomActionResponse, err error) {
	body := map[string]any{}
	err = autorest.Respond(
		resp,
		azure.WithErrorUnlessStatusCode(http.StatusOK),
		autorest.ByUnmarshallingJSON(&body),
		autorest.ByClosing())
	result.Body = body
	result.Response = autorest.Response{Response: resp}
	return
}
