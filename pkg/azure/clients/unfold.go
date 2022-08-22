// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/google/go-cmp/cmp"
	"github.com/project-radius/radius/pkg/azure/radclient"
)

// ServiceError conforms to the OData v4 error format.
// See http://docs.oasis-open.org/odata/odata-json-format/v4.0/os/odata-json-format-v4.0-os.html
//
// Note that this type is almost the same to that of azure.ServiceError, with the difference
// being the Details field having more structure.  We need that structure to unfold the
// error messages.
type ServiceError struct {
	Code           string                   `json:"code,omitempty" yaml:"code,omitempty"`
	Message        string                   `json:"message,omitempty" yaml:"message,omitempty"`
	Target         *string                  `json:"target,omitempty" yaml:"target,omitempty"`
	Details        []*radclient.ErrorDetail `json:"details,omitempty" yaml:"details,omitempty"`
	InnerError     map[string]interface{}   `json:"innererror,omitempty" yaml:"innererror,omitempty"`
	AdditionalInfo []map[string]interface{} `json:"additionalInfo,omitempty" yaml:"additionalInfo,omitempty"`
}

// UnfoldServiceError unfolds the Details field in the given azure.ServiceError,
// and convert messages which are raw JSON of an radclient.ErrorResponse into
// structured radclient.ErrorDetails fields.
//
// This is needed because for custom RP, errors are not treated as structured
// JSON, even if we follow the error format from ARM.
func UnfoldServiceError(in *azure.ServiceError) *ServiceError {
	out := &ServiceError{
		Code:           in.Code,
		Message:        in.Message,
		Target:         in.Target,
		InnerError:     in.InnerError,
		AdditionalInfo: in.AdditionalInfo,
	}
	// Now, the details defined in azure.ServiceError is unstructured, but
	// it is in fact has structure based on OData V4. We will reparse.
	if in.Details == nil {
		return out
	}
	out.Details = make([]*radclient.ErrorDetail, len(in.Details))
	for i, d := range in.Details {
		out.Details[i] = &radclient.ErrorDetail{}
		// First we attempt to deserialize this raw form to the format
		// of radclient.ErrorDetail.
		if err := roundTripJSON(d, out.Details[i]); err != nil {
			// If the deserialization didn't work, we fall back to
			// just extracting out the fields using the contract in OData V4 error.
			*out.Details[i] = radclient.ErrorDetail{
				Code:    extractString(d["code"]),
				Message: extractString(d["message"]),
				Target:  extractString(d["target"]),
			}
		}
		// Since these Details may have raw JSON in their Message field,
		// we call UnfoldErrorDetails to extract out the real detail
		// format.
		out.Details[i] = UnfoldErrorDetails(out.Details[i])
	}
	return out
}

// UnfoldErrorDetails extract the Message field of a given *radclient.ErrorDetail
// into its correspoding Details field, which is structured.
func UnfoldErrorDetails(d *radclient.ErrorDetail) *radclient.ErrorDetail {
	if d == nil {
		return nil
	}
	new := *d
	if new.Target != nil && *new.Target == "" {
		new.Target = nil
	}
	for i := range new.Details {
		new.Details[i] = UnfoldErrorDetails(new.Details[i])
	}
	if new.Message == nil {
		return &new
	}

	// Try to unmarshal as both ErrorResponse and ErrorDetail.
	// There seems to be some inconsistency in the error format returned from RPs
	// here. We will try to unmarshal as ErrorResponse first, and if it fails,
	// we will try to unmarshal as ErrorDetail.
	// Note, the difference between ErrorResponse and ErrorDetail is that ErrorResponse has an extra level of wrapping with `error`.

	// For example, Cosmos DB is returning an ErrorDetails in response to a 400 for this bicep:
	// resource cosmosDb 'mongodbDatabases' = {
	// 	name: 'db'
	// 	properties: {
	// 	  resource: {
	// 		id: 'db2'
	// 	  }
	// 	  options: {
	// 		throughput: 400
	// 	  }
	// 	}
	// }
	// "details":[{"code":"BadRequest","message":"{\r\n  \"code\": \"BadRequest\",\r\n  \"message\": \"Resource name db in request-uri does not match Resource name db2 in request-body.\\r\\nActivityId: 2c1f5342-0e09-450d-a414-196e02e16085, Microsoft.Azure.Documents.Common/2.14.0\"\r\n}"}]
	// Counter example, Cosmos DB is returning an ErrorResponse in response to a 400 when the name/id are too long
	// "details":[{"code":"BadRequest","message":"{\r\n  \"error\": {\r\n    \"code\": \"BadRequest\",\r\n    \"message\": \"<!DOCTYPE HTML PUBLIC \\\"-//W3C//DTD HTML 4.01//EN\\\"\\\"http://www.w3.org/TR/html4/strict.dtd\\\">\\r\\n<HTML><HEAD><TITLE>Bad Request</TITLE>\\r\\n<META HTTP-EQUIV=\\\"Content-Type\\\" Content=\\\"text/html; charset=us-ascii\\\"></HEAD>\\r\\n<BODY><h2>Bad Request - Invalid URL</h2>\\r\\n<hr><p>HTTP Error 400. The request URL is invalid.</p>\\r\\n</BODY></HTML>\\r\\n\"\r\n  }\r\n}"}]
	// In radius, we always return ErrorResponse for these calls.
	resp := radclient.ErrorResponse{}
	err := json.Unmarshal([]byte(*d.Message), &resp)
	if err != nil || resp.InnerError == nil || cmp.Equal(resp.InnerError, radclient.ErrorDetail{}) {
		// Try as ErrorDetail directly rather than ErrorResponse
		details := radclient.ErrorDetail{}
		err := json.Unmarshal([]byte(*d.Message), &details)

		if err != nil || cmp.Equal(details, radclient.ErrorDetail{}) {
			return &new
		}

		// We successfully parse an armerrors.ErrorDetails from the message.
		// Let's move that information into the structured details.
		new.Message = nil
		new.Details = append(new.Details, &details)
		return &new
	}
	// We successfully parse an armerrors.ErrorResponse from the message.
	// Let's move that information into the structured details.
	new.Message = nil
	new.Details = append(new.Details, UnfoldErrorDetails(resp.InnerError))
	return &new
}

// TryUnfoldErrorResponse takes an error that wrapped a radclient.ErrorResponse
// and unfold nested JSON messages into structured radclient.ErrorDetail field.
//
// If the given error isn't wrapping a *radclient.ErrorResponse, nil is returned.
func TryUnfoldErrorResponse(err error) *radclient.ErrorDetail {
	inner, ok := errors.Unwrap(err).(*radclient.ErrorResponse)
	if inner == nil || !ok {
		return nil
	}
	return UnfoldErrorDetails(inner.InnerError)
}

// TryUnfoldServiceError calls UnfoldServiceError if the given error is
// of the type ServiceError.  Otherwise, returns nil.
func TryUnfoldServiceError(err error) *ServiceError {
	svcError, ok := err.(*azure.ServiceError)
	if !ok {
		return nil
	}
	return UnfoldServiceError(svcError)
}

func roundTripJSON(input interface{}, output interface{}) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, output)
}

func extractString(o interface{}) *string {
	if o == nil {
		return nil
	}
	if s, ok := o.(string); ok {
		return &s
	}
	if sp, ok := o.(*string); ok {
		return sp
	}
	return to.StringPtr(fmt.Sprintf("%v", o))
}
