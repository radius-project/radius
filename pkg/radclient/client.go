// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/validation"
)

const apiVersion = "2018-09-01-preview"
const resourceProviderNamespace = "Microsoft.CustomProviders"
const parentResourcePath = "resourceProviders/radius"

// Client wrapper around resources.Client to extend operations on custom resource types.
type Client struct {
	resources.Client
}

// Application Any new customer facing property (tag etc) if added to the radius application, should be added here.
type Application struct {
	Name string
	ID   string
}

// NewClient creates an instance of the radclient Client.
func NewClient(subscriptionID string) Client {
	return NewClientWithBaseURI(resources.DefaultBaseURI, subscriptionID)
}

// NewClientWithBaseURI creates an instance of the radclient Client using a custom endpoint.  Use this when interacting
// with an Azure cloud that uses a non-standard base URI (sovereign clouds, Azure stack).
func NewClientWithBaseURI(baseURI string, subscriptionID string) Client {
	return Client{resources.NewClientWithBaseURI(baseURI, subscriptionID)}
}

// GetApplication get radius application details. Currently supports name and id.
func (client Client) GetApplication(ctx context.Context, resourceGroupName string, applicationName string) (Application, error) {
	id := fmt.Sprintf("/subscriptions/%v/resourceGroups/%v/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/%v", client.SubscriptionID, resourceGroupName, applicationName)
	result, err := client.Client.GetByID(ctx, id, apiVersion)
	if err != nil {
		return Application{}, err
	}

	app := Application{
		Name: *result.Name,
		ID:   *result.ID,
	}

	return app, err
}

// ListRadiusResources lists all radius resources of the specified resource type in the resource group.
// Most of the code here is replicated from resources.Get https://github.com/Azure/azure-sdk-for-go/blob/master/services/resources/mgmt/2020-10-01/resources/resources.go#L643
// TODO Currently it only supports Application return type, extend it to support all radius resource types.
func (client Client) ListRadiusResources(ctx context.Context, resourceGroupName string, resourceType string) (resources []Application, err error) {
	if !isAValidRadiusResourceType(resourceType) {
		return resources, fmt.Errorf("%s is not a supported radius resource type", resourceType)
	}

	// TODO Add tracing support

	if err := validation.Validate([]validation.Validation{
		{TargetValue: resourceGroupName,
			Constraints: []validation.Constraint{{Target: "resourceGroupName", Name: validation.MaxLength, Rule: 90, Chain: nil},
				{Target: "resourceGroupName", Name: validation.MinLength, Rule: 1, Chain: nil},
				{Target: "resourceGroupName", Name: validation.Pattern, Rule: `^[-\p{L}\._\(\)\w]+$`, Chain: nil}}}}); err != nil {
		return resources, validation.NewError("resources.Client", "Get", err.Error())
	}

	req, err := client.getResourcesPreparer(ctx, resourceGroupName, resourceType)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.Client", "Get", nil, "Failure preparing request")
		return resources, err
	}

	resp, err := client.GetSender(req)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.Client", "Get", resp, "Failure sending request")
		return []Application{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resources, wrapHTTPResponseInAzureError(resp)
	}

	// TODO this can be improved by unmarshalling directly into the struct, or rather override resources.GetResponder with support for list of resources
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resources, fmt.Errorf("failed to read the response: %w", err)
	}
	var parsedResponse map[string]interface{}
	err = json.Unmarshal(respBytes, &parsedResponse)
	if err != nil {
		return resources, err
	}
	if parsedResponse["value"] == nil {
		return []Application{}, err
	}
	applications := parsedResponse["value"].([]interface{})

	for _, value := range applications {
		app := Application{
			Name: value.(map[string]interface{})["name"].(string),
			ID:   value.(map[string]interface{})["id"].(string),
		}
		resources = append(resources, app)
	}

	return resources, err
}

// Prepares the ListRadiusResources request.
// Taken from resources.GetPreparer with added support for custom resource types. https://github.com/Azure/azure-sdk-for-go/blob/master/services/resources/mgmt/2020-10-01/resources/resources.go#L685
func (client Client) getResourcesPreparer(ctx context.Context, resourceGroupName string, resourceType string) (*http.Request, error) {
	pathParameters := map[string]interface{}{
		"parentResourcePath":        parentResourcePath,
		"resourceGroupName":         autorest.Encode("path", resourceGroupName),
		"resourceProviderNamespace": autorest.Encode("path", resourceProviderNamespace),
		"resourceType":              resourceType,
		"subscriptionId":            autorest.Encode("path", client.SubscriptionID),
	}

	queryParameters := map[string]interface{}{
		"api-version": apiVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPathParameters("/subscriptions/{subscriptionId}/resourcegroups/{resourceGroupName}/providers/{resourceProviderNamespace}/{parentResourcePath}/{resourceType}", pathParameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

func isAValidRadiusResourceType(rt string) bool {
	var supportedResourceTypes = []string{"Applications"}
	for _, st := range supportedResourceTypes {
		if st == rt {
			return true
		}
	}
	return false
}

// Wraps the response into an error. This code is mostly taken from azure.WithErrorUnlessStatusCode
func wrapHTTPResponseInAzureError(resp *http.Response) (err error) {
	var e azure.RequestError

	encodedAs := autorest.EncodedAsJSON
	if strings.Contains(resp.Header.Get("Content-Type"), "xml") {
		encodedAs = autorest.EncodedAsXML
	}

	// Copy and replace the Body in case it does not contain an error object.
	// This will leave the Body available to the caller.
	b, decodeErr := autorest.CopyAndDecode(encodedAs, resp.Body, &e)
	resp.Body = ioutil.NopCloser(&b)
	if decodeErr != nil {
		return fmt.Errorf("error response cannot be parsed: %q error: %v", b.String(), decodeErr)
	}
	if e.ServiceError == nil {
		// Check if error is unwrapped ServiceError
		decoder := autorest.NewDecoder(encodedAs, bytes.NewReader(b.Bytes()))
		if err := decoder.Decode(&e.ServiceError); err != nil {
			return fmt.Errorf("error response cannot be parsed: %q error: %v", b.String(), err)
		}

		// should the API return the literal value `null` as the response
		if e.ServiceError == nil {
			e.ServiceError = &azure.ServiceError{
				Code:    "Unknown",
				Message: "Unknown service error",
				Details: []map[string]interface{}{
					{
						"HttpResponse.Body": b.String(),
					},
				},
			}
		}
	}

	if e.ServiceError != nil && e.ServiceError.Message == "" {
		// if we're here it means the returned error wasn't OData v4 compliant.
		// try to unmarshal the body in hopes of getting something.
		rawBody := map[string]interface{}{}
		decoder := autorest.NewDecoder(encodedAs, bytes.NewReader(b.Bytes()))
		if err := decoder.Decode(&rawBody); err != nil {
			return fmt.Errorf("Error response cannot be parsed: %q error: %v", b.String(), err)
		}

		e.ServiceError = &azure.ServiceError{
			Code:    "Unknown",
			Message: "Unknown service error",
		}
		if len(rawBody) > 0 {
			e.ServiceError.Details = []map[string]interface{}{rawBody}
		}
	}

	e.Response = resp
	e.RequestID = azure.ExtractRequestID(resp)
	if e.StatusCode == nil {
		e.StatusCode = resp.StatusCode
	}
	err = &e

	return err
}
