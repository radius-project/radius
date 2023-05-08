/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package clientv2

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/google/go-cmp/cmp"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/to"
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
	Details        []*generated.ErrorDetail `json:"details,omitempty" yaml:"details,omitempty"`
	InnerError     map[string]any           `json:"innererror,omitempty" yaml:"innererror,omitempty"`
	AdditionalInfo []map[string]any         `json:"additionalInfo,omitempty" yaml:"additionalInfo,omitempty"`
}

// UnfoldServiceError unfolds the Details field in the given azure.ServiceError,
// and convert messages which are raw JSON of an radclient.ErrorResponse into
// structured radclient.ErrorDetails fields.
//
// This is needed because for custom RP, errors are not treated as structured
// JSON, even if we follow the error format from ARM.
func UnfoldServiceError(in *generated.ErrorDetail) *ServiceError {
	out := &ServiceError{
		Code:    *in.Code,
		Message: *in.Message,
		Target:  in.Target,
		// InnerError:     in.Details,
		// AdditionalInfo: in.AdditionalInfo,
	}
	// Now, the details defined in azure.ServiceError is unstructured, but
	// it is in fact has structure based on OData V4. We will reparse.
	if in.Details == nil {
		return out
	}
	out.Details = make([]*generated.ErrorDetail, len(in.Details))
	for i, d := range in.Details {
		out.Details[i] = &generated.ErrorDetail{}
		// First we attempt to deserialize this raw form to the format
		// of armerrors.ErrorDetail.
		if err := roundTripJSON(d, out.Details[i]); err != nil {
			// If the deserialization didn't work, we fall back to
			// just extracting out the fields using the contract in OData V4 error.
			*out.Details[i] = generated.ErrorDetail{
				Code:    extractString(d.Code),
				Message: extractString(d.Message),
				Target:  extractString(d.Target),
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
func UnfoldErrorDetails(d *generated.ErrorDetail) generated.ErrorDetail {
	if d == nil {
		return generated.ErrorDetail{}
	}

	new := *d
	if new.Target != nil {
		new.Target = nil
	}

	for i := range new.Details {
		errorDetails := UnfoldErrorDetails(new.Details[i])
		new.Details[i] = &errorDetails
	}

	if new.Message == nil {
		return new
	}

	resp := &generated.ErrorResponse{}
	err := json.Unmarshal([]byte(*d.Message), &resp)
	if err != nil || cmp.Equal(resp.Error, v1.ErrorDetails{}) {
		return new
	}

	// We successfully parse an v1.ErrorResponse from the message.
	// Let's move that information into the structured details.
	new.Message = nil
	errorDetails := UnfoldErrorDetails(resp.Error)
	new.Details = append(new.Details, &errorDetails)
	return new
}

type WrappedErrorResponse struct {
	ErrorResponse generated.ErrorResponse
}

func (w WrappedErrorResponse) Error() string {
	return *w.ErrorResponse.Error.Message
}

// TryUnfoldErrorResponse takes an error that wrapped a radclient.ErrorResponse
// and unfold nested JSON messages into structured radclient.ErrorDetail field.
//
// If the given error isn't wrapping a *radclient.ErrorResponse, nil is returned.
func TryUnfoldErrorResponse(err error) *generated.ErrorDetail {
	inner, ok := errors.Unwrap(err).(WrappedErrorResponse)
	if cmp.Equal(inner.ErrorResponse.Error, v1.ErrorDetails{}) || !ok {
		return nil
	}

	errDetails := UnfoldErrorDetails(inner.ErrorResponse.Error)
	return &errDetails
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

// TryUnfoldResponseError unfolds an azcore.ResponseError.
func TryUnfoldResponseError(err error) *v1.ErrorDetails {
	respErr, ok := err.(*azcore.ResponseError)
	if !ok {
		return nil
	}
	return unfoldResponseError(respErr)
}

func unfoldResponseError(in *azcore.ResponseError) *v1.ErrorDetails {
	if in == nil {
		return nil
	}

	body, err := readResponseBody(in.RawResponse)
	if err != nil {
		return nil
	}

	raw := struct {
		Error v1.ErrorDetails `json:"error"`
	}{}

	err = json.Unmarshal(body, &raw)
	if err != nil {
		return nil
	}

	return &raw.Error
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp.Body == nil {
		return []byte{}, nil
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	return data, nil
}
