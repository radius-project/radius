// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httpbaseline

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
)

// headersToExclude is a set of header values to skip in test comparisons. These either vary per-request or add noise.
var headersToExclude = map[string]bool{
	"User-Agent":      true,
	"X-Forwarded-For": true,
}

// Request represents an http.Request that we can store on disk.
type Request struct {
	URL     string              `json:"url"`
	Method  string              `json:"method"`
	Headers map[string][]string `json:"headers"`

	// Right now we assume that the body is text
	Body string `json:"body"`
}

// Response represents an http.Response that we can store on disk.
type Response struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers"`

	// Right now we assume that the body is text
	Body string `json:"body"`
}

func NewRequest(r *http.Request) (*Request, error) {
	request := Request{
		Headers: map[string][]string{},
		Method:  r.Method,
		URL:     r.URL.String(),
	}

	for k, v := range r.Header {
		_, skip := headersToExclude[k]

		if !skip {
			request.Headers[k] = v
		}
	}

	// Buffer and replace the body so it can be read again if we need to.
	if r.Body != nil {
		buf := bytes.Buffer{}
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			return nil, err
		}
		err = r.Body.Close()
		if err != nil {
			return nil, err
		}

		request.Body = buf.String()
		r.Body = ioutil.NopCloser(&buf)
	}

	return &request, nil
}

func NewResponse(r *http.Response) (*Response, error) {
	response := Response{
		Headers:    map[string][]string{},
		StatusCode: r.StatusCode,
	}

	for k, v := range r.Header {
		_, skip := headersToExclude[k]

		if !skip {
			response.Headers[k] = v
		}
	}

	// Buffer and replace the body so it can be read again if we need to.
	if r.Body != nil {
		buf := bytes.Buffer{}
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			return nil, err
		}
		err = r.Body.Close()
		if err != nil {
			return nil, err
		}

		response.Body = buf.String()
		r.Body = ioutil.NopCloser(&buf)
	}

	return &response, nil
}

func ReadRequestFromFile(path string) (Request, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return Request{}, err
	}

	request := Request{}
	err = json.Unmarshal(b, &request)
	if err != nil {
		return Request{}, err
	}

	return request, nil
}

func ReadResponseFromFile(path string) (Response, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return Response{}, err
	}

	response := Response{}
	err = json.Unmarshal(b, &response)
	if err != nil {
		return Response{}, err
	}

	return response, nil
}

func (req Request) ToTestRequest(ctx context.Context) *http.Request {
	result := httptest.NewRequest(req.Method, req.URL, bytes.NewBufferString(req.Body))
	result = result.WithContext(ctx)
	for key, values := range req.Headers {
		result.Header[key] = values
	}

	return result
}

func (res Response) CreateRoundTripper() *RoundTripper {
	return &RoundTripper{
		Response: res,
	}
}

// RoundTripper is a single-use http.RoundTripper implementation that can capture
// a request and produces a pre-configured response.
type RoundTripper struct {
	Request  Request
	Response Response
}

var _ http.RoundTripper = (*RoundTripper)(nil)

func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Capture and respond.
	captured, err := NewRequest(req)
	if err != nil {
		return nil, err
	}

	rt.Request = *captured

	w := httptest.NewRecorder()
	w.Header()

	for key, values := range rt.Response.Headers {
		w.Header()[key] = values
	}

	w.WriteHeader(w.Code)
	_, err = w.WriteString(rt.Request.Body)
	if err != nil {
		return nil, err
	}

	result := w.Result()
	result.Request = req
	return result, nil
}
