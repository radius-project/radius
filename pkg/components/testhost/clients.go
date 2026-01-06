/*
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
*/

package testhost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/stretchr/testify/require"
)

// TestResponse is returned from requests made against a TestHost. Tests should use the functions defined
// on TestResponse for validation.
type TestResponse struct {
	// Raw is the raw HTTP response.
	Raw *http.Response

	// Body is the response body.
	Body *bytes.Buffer

	// Error is the ARM error response if the response status code is >= 400.
	Error *v1.ErrorResponse

	// t is the test object.
	t *testing.T

	// host is the TestHost that served this response.
	host *TestHost
}

// MakeFixtureRequest sends a request to the server using a file on disk as the payload (body). Use the fixture
// parameter to specify the path to a file.
func (th *TestHost) MakeFixtureRequest(method string, pathAndQuery string, fixture string) *TestResponse {
	body, err := os.ReadFile(fixture)
	require.NoError(th.t, err, "reading fixture failed")
	return th.MakeRequest(method, pathAndQuery, body)
}

// MakeTypedRequest sends a request to the server by marshalling the provided object to JSON.
func (th *TestHost) MakeTypedRequest(method string, pathAndQuery string, body any) *TestResponse {
	if body == nil {
		return th.MakeRequest(method, pathAndQuery, nil)
	}

	b, err := json.Marshal(body)
	require.NoError(th.t, err, "marshalling body failed")
	return th.MakeRequest(method, pathAndQuery, b)
}

// MakeRequest sends a request to the server.
func (th *TestHost) MakeRequest(method string, pathAndQuery string, body []byte) *TestResponse {
	// Prepend the base path if this is a relative URL.
	requestUrl := pathAndQuery
	parsed, err := url.Parse(pathAndQuery)
	require.NoError(th.t, err, "parsing URL failed")
	if !parsed.IsAbs() {
		requestUrl = th.BaseURL() + pathAndQuery
	}

	client := th.Client()
	request, err := rpctest.NewHTTPRequestWithContent(context.Background(), method, requestUrl, body)
	require.NoError(th.t, err, "creating request failed")

	ctx := rpctest.NewARMRequestContext(request)
	request = request.WithContext(ctx)

	response, err := client.Do(request)
	require.NoError(th.t, err, "sending request failed")

	// Buffer the response so we can read multiple times.
	responseBuffer := &bytes.Buffer{}
	_, err = io.Copy(responseBuffer, response.Body)
	response.Body.Close()
	require.NoError(th.t, err, "copying response failed")

	response.Body = io.NopCloser(responseBuffer)

	// Pretty-print response for logs.
	if len(responseBuffer.Bytes()) > 0 {
		var data any
		err = json.Unmarshal(responseBuffer.Bytes(), &data)
		require.NoError(th.t, err, "unmarshalling response failed")

		text, err := json.MarshalIndent(&data, "", "  ")
		require.NoError(th.t, err, "marshalling response failed")
		th.t.Log("Response Body: \n" + string(text))
	}

	var errorResponse *v1.ErrorResponse
	if response.StatusCode >= 400 {
		// The response MUST be an arm error for a non-success status code.
		errorResponse = &v1.ErrorResponse{}
		err := json.Unmarshal(responseBuffer.Bytes(), &errorResponse)
		require.NoError(th.t, err, "unmarshalling error response failed - THIS IS A SERIOUS BUG. ALL ERROR RESPONSES MUST USE THE STANDARD FORMAT")
	}

	return &TestResponse{Raw: response, Body: responseBuffer, Error: errorResponse, host: th, t: th.t}
}

// EqualsErrorCode compares a TestResponse against an expected status code and error code. EqualsErrorCode assumes the response
// uses the ARM error format (required for our APIs).
func (tr *TestResponse) EqualsErrorCode(statusCode int, code string) {
	require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
	require.NotNil(tr.t, tr.Error, "expected an error but actual response did not contain one")
	require.Equal(tr.t, code, tr.Error.Error.Code, "actual error code was different from expected")
}

// EqualsFixture compares a TestResponse against an expected status code and body payload. Use the fixture parameter to specify
// the path to a file.
func (tr *TestResponse) EqualsFixture(statusCode int, fixture string) {
	body, err := os.ReadFile(fixture)
	require.NoError(tr.t, err, "reading fixture failed")
	tr.EqualsResponse(statusCode, body)
}

// EqualsStatusCode compares a TestResponse against an expected status code (ingnores the body payload).
func (tr *TestResponse) EqualsStatusCode(statusCode int) {
	require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
}

// EqualsFixture compares a TestResponse against an expected status code and body payload.
func (tr *TestResponse) EqualsResponse(statusCode int, body []byte) {
	if len(body) == 0 {
		require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
		require.Empty(tr.t, tr.Body.Bytes(), "expected an empty response but actual response had a body")
		return
	}

	var expected map[string]any
	err := json.Unmarshal(body, &expected)
	require.NoError(tr.t, err, "unmarshalling expected response failed")

	var actual map[string]any
	err = json.Unmarshal(tr.Body.Bytes(), &actual)

	tr.removeSystemData(actual)

	require.NoError(tr.t, err, "unmarshalling actual response failed. Got '%v'", tr.Body.String())
	require.EqualValues(tr.t, expected, actual, "response body did not match expected")
	require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
}

// EqualsValue compares a TestResponse against an expected status code and an response body.
//
// If the systemData propert is present in the response, it will be removed.
func (tr *TestResponse) EqualsValue(statusCode int, expected any) {
	var actual map[string]any
	err := json.Unmarshal(tr.Body.Bytes(), &actual)
	require.NoError(tr.t, err, "unmarshalling actual response failed")

	// Convert expected input to map[string]any to compare with actual response.
	expectedBytes, err := json.Marshal(expected)
	require.NoError(tr.t, err, "marshalling expected response failed")

	var expectedMap map[string]any
	err = json.Unmarshal(expectedBytes, &expectedMap)
	require.NoError(tr.t, err, "unmarshalling expected response failed")

	tr.removeSystemData(expectedMap)
	tr.removeSystemData(actual)

	require.EqualValues(tr.t, expectedMap, actual, "response body did not match expected")
	require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
}

// EqualsEmptyList compares a TestResponse against an expected status code and an empty resource list.
func (tr *TestResponse) EqualsEmptyList() {
	expected := map[string]any{
		"value": []any{},
	}

	var actual map[string]any
	err := json.Unmarshal(tr.Body.Bytes(), &actual)

	tr.removeSystemData(actual)

	require.NoError(tr.t, err, "unmarshalling actual response failed")
	require.EqualValues(tr.t, expected, actual, "response body did not match expected")
	require.Equal(tr.t, http.StatusOK, tr.Raw.StatusCode, "status code did not match expected")
}

func (tr *TestResponse) ReadAs(obj any) {
	tr.t.Helper()

	decoder := json.NewDecoder(tr.Body)
	decoder.DisallowUnknownFields()

	err := decoder.Decode(obj)
	require.NoError(tr.t, err, "unmarshalling expected response failed")
}

func (tr *TestResponse) WaitForOperationComplete(timeout *time.Duration) *TestResponse {
	if tr.Raw.StatusCode != http.StatusCreated && tr.Raw.StatusCode != http.StatusAccepted {
		// Response is already terminal.
		return tr
	}

	if timeout == nil {
		x := 30 * time.Second
		timeout = &x
	}

	timer := time.After(*timeout)
	poller := time.NewTicker(1 * time.Second)
	defer poller.Stop()
	for {
		select {
		case <-timer:
			tr.t.Fatalf("timed out waiting for operation to complete")
			return nil // unreachable
		case <-poller.C:
			// The Location header should give us the operation status URL.
			response := tr.host.MakeRequest(http.MethodGet, tr.Raw.Header.Get("Azure-AsyncOperation"), nil)

			// To determine if the response is terminal we need to read the provisioning state field.
			operationStatus := v1.AsyncOperationStatus{}
			response.ReadAs(&operationStatus)
			if operationStatus.Status.IsTerminal() {
				// Response is terminal.
				return response
			}

			continue
		}
	}
}

func (tr *TestResponse) removeSystemData(responseBody map[string]any) {
	// Delete systemData property if found, it's not stable so we don't include it in baselines.
	_, ok := responseBody["systemData"]
	if ok {
		delete(responseBody, "systemData")
		return
	}

	value, ok := responseBody["value"]
	if !ok {
		return
	}

	valueSlice, ok := value.([]any)
	if !ok {
		return
	}

	for _, v := range valueSlice {
		if vMap, ok := v.(map[string]any); ok {
			tr.removeSystemData(vMap)
		}
	}
}
