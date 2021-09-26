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
	Body     map[string]interface{}
	Response autorest.Response
}

func (client CustomActionClient) InvokeCustomAction(ctx context.Context, id string, apiVersion string, action string) (result CustomActionResponse, err error) {
	req, err := client.InvokeCustomActionPreparer(ctx, id, apiVersion, action)
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

func (client CustomActionClient) InvokeCustomActionPreparer(ctx context.Context, id string, apiVersion string, action string) (*http.Request, error) {
	pathParameters := map[string]interface{}{
		"id":     autorest.Encode("path", id),
		"action": autorest.Encode("path", action),
	}

	queryParameters := map[string]interface{}{
		"api-version": apiVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPost(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPathParameters("{id}/{action}", pathParameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

func (client CustomActionClient) InvokeCustomActionSender(req *http.Request) (*http.Response, error) {
	return client.Send(req, azure.DoRetryWithRegistration(client.Client))
}

func (client CustomActionClient) InvokeCustomActionResponder(resp *http.Response) (result CustomActionResponse, err error) {
	body := map[string]interface{}{}
	err = autorest.Respond(
		resp,
		azure.WithErrorUnlessStatusCode(http.StatusOK),
		autorest.ByUnmarshallingJSON(&body),
		autorest.ByClosing())
	result.Body = body
	result.Response = autorest.Response{Response: resp}
	return
}
