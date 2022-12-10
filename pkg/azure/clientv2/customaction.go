// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
)

const (
	DefaultBaseURI = "https://management.azure.com"
)

type BaseClient struct {
	armresources.Client
	BaseURI string
}

type CustomActionClient struct {
	BaseClient
}

func New(subscriptionID string, credential azcore.TokenCredential) (*BaseClient, error) {
	client, err := NewWithBaseURI(DefaultBaseURI, subscriptionID, credential)
	if err != nil {
		return nil, err
	}

	return client, err
}

func NewWithBaseURI(baseURI string, subscriptionID string, credential azcore.TokenCredential) (*BaseClient, error) {
	// FIXME: armresources.Client doesn't accept BaseURI. How can I use that?
	client, err := armresources.NewClient(subscriptionID, credential, &arm.ClientOptions{})
	if err != nil {
		return nil, err
	}

	return &BaseClient{
		Client:  *client,
		BaseURI: baseURI,
	}, nil
}

type CustomActionResponse struct {
	Body     map[string]interface{}
	Response autorest.Response
}

func (client CustomActionClient) InvokeCustomAction(ctx context.Context, id string, apiVersion string, action string, body interface{}) (result CustomActionResponse, err error) {
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

func (client CustomActionClient) InvokeCustomActionPreparer(ctx context.Context, id string, apiVersion string, action string, body interface{}) (*http.Request, error) {
	pathParameters := map[string]interface{}{
		"id":     autorest.Encode("none", id),
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
	if body != nil {
		preparer = autorest.DecoratePreparer(preparer, autorest.WithJSON(body))
	}

	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

func (client CustomActionClient) InvokeCustomActionSender(req *http.Request) (*http.Response, error) {
	// return client.Send(req, azure.DoRetryWithRegistration(client.Client))
	return nil, nil
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
