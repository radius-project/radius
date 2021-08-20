// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azclients

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/google/go-cmp/cmp"
)

// ServiceError conforms to the OData v4 error format.
// See http://docs.oasis-open.org/odata/odata-json-format/v4.0/os/odata-json-format-v4.0-os.html
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
	resp := radclient.ErrorResponse{}
	err := json.Unmarshal([]byte(*d.Message), &resp)
	if err != nil || cmp.Equal(resp.InnerError, radclient.ErrorDetail{}) {
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
