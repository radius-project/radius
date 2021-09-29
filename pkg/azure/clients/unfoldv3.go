// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"encoding/json"
	"errors"

	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/radius/pkg/azure/radclientv3"
	"github.com/google/go-cmp/cmp"
)

// ServiceErrorV3 conforms to the OData v4 error format.
// See http://docs.oasis-open.org/odata/odata-json-format/v4.0/os/odata-json-format-v4.0-os.html
//
// Note that this type is almost the same to that of azure.ServiceError, with the difference
// being the Details field having more structure.  We need that structure to unfold the
// error messages.
type ServiceErrorV3 struct {
	Code           string                     `json:"code,omitempty" yaml:"code,omitempty"`
	Message        string                     `json:"message,omitempty" yaml:"message,omitempty"`
	Target         *string                    `json:"target,omitempty" yaml:"target,omitempty"`
	Details        []*radclientv3.ErrorDetail `json:"details,omitempty" yaml:"details,omitempty"`
	InnerError     map[string]interface{}     `json:"innererror,omitempty" yaml:"innererror,omitempty"`
	AdditionalInfo []map[string]interface{}   `json:"additionalInfo,omitempty" yaml:"additionalInfo,omitempty"`
}

// UnfoldServiceErrorV3 unfolds the Details field in the given azure.ServiceError,
// and convert messages which are raw JSON of an radclientv3.ErrorResponse into
// structured radclientv3.ErrorDetails fields.
//
// This is needed because for custom RP, errors are not treated as structured
// JSON, even if we follow the error format from ARM.
func UnfoldServiceErrorV3(in *azure.ServiceError) *ServiceErrorV3 {
	out := &ServiceErrorV3{
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
	out.Details = make([]*radclientv3.ErrorDetail, len(in.Details))
	for i, d := range in.Details {
		out.Details[i] = &radclientv3.ErrorDetail{}
		// First we attempt to deserialize this raw form to the format
		// of radclientv3.ErrorDetail.
		if err := roundTripJSON(d, out.Details[i]); err != nil {
			// If the deserialization didn't work, we fall back to
			// just extracting out the fields using the contract in OData V4 error.
			*out.Details[i] = radclientv3.ErrorDetail{
				Code:    extractString(d["code"]),
				Message: extractString(d["message"]),
				Target:  extractString(d["target"]),
			}
		}
		// Since these Details may have raw JSON in their Message field,
		// we call UnfoldErrorDetails to extract out the real detail
		// format.
		out.Details[i] = UnfoldErrorDetailsV3(out.Details[i])
	}
	return out
}

// UnfoldErrorDetails extract the Message field of a given *radclientv3.ErrorDetail
// into its correspoding Details field, which is structured.
func UnfoldErrorDetailsV3(d *radclientv3.ErrorDetail) *radclientv3.ErrorDetail {
	if d == nil {
		return nil
	}
	new := *d
	if new.Target != nil && *new.Target == "" {
		new.Target = nil
	}
	for i := range new.Details {
		new.Details[i] = UnfoldErrorDetailsV3(new.Details[i])
	}
	if new.Message == nil {
		return &new
	}
	resp := radclientv3.ErrorResponse{}
	err := json.Unmarshal([]byte(*d.Message), &resp)
	if err != nil || cmp.Equal(resp.InnerError, radclientv3.ErrorDetail{}) {
		return &new
	}
	// We successfully parse an armerrors.ErrorResponse from the message.
	// Let's move that information into the structured details.
	new.Message = nil
	new.Details = append(new.Details, UnfoldErrorDetailsV3(resp.InnerError))
	return &new
}

// TryUnfoldErrorResponse takes an error that wrapped a radclientv3.ErrorResponse
// and unfold nested JSON messages into structured radclientv3.ErrorDetail field.
//
// If the given error isn't wrapping a *radclientv3.ErrorResponse, nil is returned.
func TryUnfoldErrorResponseV3(err error) *radclientv3.ErrorDetail {
	inner, ok := errors.Unwrap(err).(*radclientv3.ErrorResponse)
	if inner == nil || !ok {
		return nil
	}
	return UnfoldErrorDetailsV3(inner.InnerError)
}

// TryUnfoldServiceErrorV3 calls UnfoldServiceErrorV3 if the given error is
// of the type ServiceErrorV3.  Otherwise, returns nil.
func TryUnfoldServiceErrorV3(err error) *ServiceErrorV3 {
	svcError, ok := err.(*azure.ServiceError)
	if !ok {
		return nil
	}
	return UnfoldServiceErrorV3(svcError)
}
