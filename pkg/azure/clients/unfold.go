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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

// ServiceError conforms to the OData v4 error format.
// See http://docs.oasis-open.org/odata/odata-json-format/v4.0/os/odata-json-format-v4.0-os.html
//
// Note that this type is almost the same to that of azure.ServiceError, with the difference
// being the Details field having more structure.  We need that structure to unfold the
// error messages.
type ServiceError struct {
	Code           string             `json:"code,omitempty" yaml:"code,omitempty"`
	Message        string             `json:"message,omitempty" yaml:"message,omitempty"`
	Target         *string            `json:"target,omitempty" yaml:"target,omitempty"`
	Details        []*v1.ErrorDetails `json:"details,omitempty" yaml:"details,omitempty"`
	InnerError     map[string]any     `json:"innererror,omitempty" yaml:"innererror,omitempty"`
	AdditionalInfo []map[string]any   `json:"additionalInfo,omitempty" yaml:"additionalInfo,omitempty"`
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
	out.Details = make([]*v1.ErrorDetails, len(in.Details))
	for i, d := range in.Details {
		out.Details[i] = &v1.ErrorDetails{}
		// First we attempt to deserialize this raw form to the format
		// of armerrors.ErrorDetail.
		if err := roundTripJSON(d, out.Details[i]); err != nil {
			// If the deserialization didn't work, we fall back to
			// just extracting out the fields using the contract in OData V4 error.
			*out.Details[i] = v1.ErrorDetails{
				Code:    *extractString(d["code"]),
				Message: *extractString(d["message"]),
				Target:  *extractString(d["target"]),
			}
		}
		// Since these Details may have raw JSON in their Message field,
		// we call UnfoldErrorDetails to extract out the real detail
		// format.
		errDetails := UnfoldErrorDetails(out.Details[i])
		out.Details[i] = &errDetails
	}
	return out
}

// UnfoldErrorDetails extract the Message field of a given *radclient.ErrorDetail
// into its correspoding Details field, which is structured.
func UnfoldErrorDetails(d *v1.ErrorDetails) v1.ErrorDetails {
	if d == nil {
		return v1.ErrorDetails{}
	}

	new := *d
	if new.Target != "" {
		new.Target = ""
	}

	for i := range new.Details {
		new.Details[i] = UnfoldErrorDetails(&new.Details[i])
	}

	if new.Message == "" {
		return new
	}

	resp := v1.ErrorResponse{}
	err := json.Unmarshal([]byte(d.Message), &resp)
	if err != nil || cmp.Equal(resp.Error, v1.ErrorDetails{}) {
		return new
	}

	// We successfully parse an v1.ErrorResponse from the message.
	// Let's move that information into the structured details.
	new.Message = ""
	new.Details = append(new.Details, UnfoldErrorDetails(&resp.Error))
	return new
}

type WrappedErrorResponse struct {
	ErrorResponse v1.ErrorResponse
}

func (w WrappedErrorResponse) Error() string {
	return w.ErrorResponse.Error.Message
}

// TryUnfoldErrorResponse takes an error that wrapped a radclient.ErrorResponse
// and unfold nested JSON messages into structured radclient.ErrorDetail field.
//
// If the given error isn't wrapping a *radclient.ErrorResponse, nil is returned.
func TryUnfoldErrorResponse(err error) *v1.ErrorDetails {
	inner, ok := errors.Unwrap(err).(WrappedErrorResponse)
	if cmp.Equal(inner.ErrorResponse.Error, v1.ErrorDetails{}) || !ok {
		return nil
	}

	errDetails := UnfoldErrorDetails(&inner.ErrorResponse.Error)
	return &errDetails
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

func roundTripJSON(input any, output any) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, output)
}

func extractString(o any) *string {
	if o == nil {
		return nil
	}
	if s, ok := o.(string); ok {
		return &s
	}
	if sp, ok := o.(*string); ok {
		return sp
	}
	return to.Ptr(fmt.Sprintf("%v", o))
}
