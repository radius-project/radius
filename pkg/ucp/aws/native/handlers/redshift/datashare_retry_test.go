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

package redshift

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

const (
	dataShareRetryArn      = "arn:aws:redshift:us-west-2:123456789012:datashare:11111111-1111-1111-1111-111111111111/sales"
	dataShareRetryConsumer = "444455556666"
	dataShareRetryNs       = "arn:aws:redshift-serverless:us-west-2:444455556666:namespace/22222222-2222-2222-2222-222222222222"
)

// dataShareRetryResponse is a response of the stub transport: a status and an XML body, or a connection reset, as
// when AWS accepted a call and its response was lost.
type dataShareRetryResponse struct {
	status int
	body   string
	reset  bool
}

// dataShareRetryTransport answers each operation with its responses in order, and counts the calls.
type dataShareRetryTransport struct {
	mu        sync.Mutex
	responses map[string][]dataShareRetryResponse
	calls     map[string]int
}

func (s *dataShareRetryTransport) Do(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op := awsmiddleware.GetOperationName(req.Context())
	n := s.calls[op]
	s.calls[op]++
	list := s.responses[op]
	if n >= len(list) {
		return nil, fmt.Errorf("unexpected call %d of %s", n+1, op)
	}
	r := list[n]
	if r.reset {
		return nil, &dataShareResetError{}
	}
	return &http.Response{
		StatusCode: r.status,
		Header:     http.Header{"Content-Type": []string{"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Request:    req,
	}, nil
}

// dataShareResetError is the error of a connection AWS reset after it got the request, which the SDK retries.
type dataShareResetError struct{}

func (*dataShareResetError) Error() string { return "read tcp: connection reset by peer" }
func (*dataShareResetError) Unwrap() error { return syscall.ECONNRESET }

// newDataShareRetryAPI returns the real Redshift client, retrying at once, against the stub transport.
func newDataShareRetryAPI(responses map[string][]dataShareRetryResponse) (*redshift.Client, *dataShareRetryTransport) {
	stub := &dataShareRetryTransport{responses: responses, calls: map[string]int{}}
	api := redshift.NewFromConfig(aws.Config{
		Region:      dataShareRegionName,
		Credentials: credentials.NewStaticCredentialsProvider("AKIAREPLAY", "replay", ""),
		HTTPClient:  stub,
		Retryer: func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.Backoff = retry.BackoffDelayerFunc(func(int, error) (time.Duration, error) { return 0, nil })
			})
		},
	})
	return api, stub
}

// dataShareXML returns a response of the operation with the data share and its associations.
func dataShareXML(op string, list bool, associations string) string {
	share := `<DataShareArn>` + dataShareRetryArn + `</DataShareArn><ProducerArn>arn:aws:redshift-serverless:us-west-2:123456789012:namespace/11111111-1111-1111-1111-111111111111</ProducerArn>` +
		`<DataShareAssociations>` + associations + `</DataShareAssociations>`
	if list {
		share = `<DataShares><member>` + share + `</member></DataShares>`
	}
	return `<` + op + `Response xmlns="http://redshift.amazonaws.com/doc/2012-12-01/"><` + op + `Result>` + share + `</` + op + `Result></` + op + `Response>`
}

func dataShareMember(consumer string, region string, status string) string {
	r := ""
	if region != "" {
		r = `<ConsumerRegion>` + region + `</ConsumerRegion>`
	}
	return `<member><ConsumerIdentifier>` + consumer + `</ConsumerIdentifier>` + r + `<Status>` + status + `</Status><ProducerAllowedWrites>false</ProducerAllowedWrites><ConsumerAcceptedWrites>false</ConsumerAcceptedWrites></member>`
}

func dataShareFaultXML(message string) string {
	return `<ErrorResponse xmlns="http://redshift.amazonaws.com/doc/2012-12-01/"><Error><Type>Sender</Type><Code>InvalidDataShareFault</Code><Message>` + message +
		`</Message></Error><RequestId>00000000-0000-0000-0000-000000000000</RequestId></ErrorResponse>`
}

// Test_DataShare_RetryAfterLostResponse sends each call through the real SDK, which retries after AWS accepted it
// and the response was lost. AWS answers a repeated authorize or associate with the same resource, so the retry
// succeeds. It answers a repeated deauthorize or disassociate with InvalidDataShareFault (observed in a run against
// real AWS), so the handler reads again, finds the resource gone, and reports the delete done, not rejected.
func Test_DataShare_RetryAfterLostResponse(t *testing.T) {
	reset := dataShareRetryResponse{reset: true}
	ok := func(op string, associations string) dataShareRetryResponse {
		return dataShareRetryResponse{status: http.StatusOK, body: dataShareXML(op, false, associations)}
	}
	described := func(op string, associations string) dataShareRetryResponse {
		return dataShareRetryResponse{status: http.StatusOK, body: dataShareXML(op, true, associations)}
	}
	authorized := dataShareMember(dataShareRetryConsumer, "", "AUTHORIZED")
	associated := dataShareMember(dataShareRetryConsumer, "", "AVAILABLE") + dataShareMember(dataShareRetryNs, "us-west-2", "ACTIVE")
	authorization := dataShareRetryArn + "|" + dataShareRetryConsumer
	association := dataShareRetryArn + "|" + dataShareRetryNs

	tests := []struct {
		name      string
		op        string
		responses map[string][]dataShareRetryResponse
		call      func(api *redshift.Client) error
	}{
		{
			name:      "authorize",
			op:        "AuthorizeDataShare",
			responses: map[string][]dataShareRetryResponse{"AuthorizeDataShare": {reset, ok("AuthorizeDataShare", authorized)}},
			call: func(api *redshift.Client) error {
				_, err := NewDataShareAuthorization(api).Create(t.Context(), native.CreateRequest{Region: dataShareRegionName,
					Properties: map[string]any{"DataShareArn": dataShareRetryArn, "ConsumerIdentifier": dataShareRetryConsumer}})
				return err
			},
		},
		{
			name: "deauthorize",
			op:   "DeauthorizeDataShare",
			responses: map[string][]dataShareRetryResponse{
				"DescribeDataShares": {described("DescribeDataShares", authorized), described("DescribeDataShares", dataShareMember(dataShareRetryConsumer, "", "DEAUTHORIZED"))},
				"DeauthorizeDataShare": {reset, {status: http.StatusBadRequest,
					body: dataShareFaultXML("You can't remove authorization because you have already removed authorization from the data share.")}},
			},
			call: func(api *redshift.Client) error {
				return NewDataShareAuthorization(api).Delete(t.Context(), dataShareRegionName, authorization)
			},
		},
		{
			name: "associate",
			op:   "AssociateDataShareConsumer",
			responses: map[string][]dataShareRetryResponse{
				"DescribeDataSharesForConsumer": {described("DescribeDataSharesForConsumer", dataShareMember(dataShareRetryConsumer, "", "AVAILABLE"))},
				"AssociateDataShareConsumer":    {reset, ok("AssociateDataShareConsumer", associated)},
			},
			call: func(api *redshift.Client) error {
				_, err := NewDataShareConsumerAssociation(api).Create(t.Context(), native.CreateRequest{Region: dataShareRegionName,
					Properties: map[string]any{"DataShareArn": dataShareRetryArn, "ConsumerArn": dataShareRetryNs}})
				return err
			},
		},
		{
			name: "disassociate",
			op:   "DisassociateDataShareConsumer",
			responses: map[string][]dataShareRetryResponse{
				"DescribeDataSharesForConsumer": {described("DescribeDataSharesForConsumer", associated), described("DescribeDataSharesForConsumer", dataShareMember(dataShareRetryConsumer, "", "AVAILABLE"))},
				"DisassociateDataShareConsumer": {reset, {status: http.StatusBadRequest,
					body: dataShareFaultXML("You can't disassociate the data share because you have already disassociated the data share.")}},
			},
			call: func(api *redshift.Client) error {
				return NewDataShareConsumerAssociation(api).Delete(t.Context(), dataShareRegionName, association)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api, stub := newDataShareRetryAPI(tt.responses)
			err := tt.call(api)
			require.Equal(t, 2, stub.calls[tt.op], "the SDK sends the call again after the lost response")
			if strings.HasPrefix(tt.op, "De") || strings.HasPrefix(tt.op, "Dis") {
				require.ErrorIs(t, err, native.ErrNotFound, "a repeat AWS answers with InvalidDataShareFault is a finished delete")
				var failure *native.Failure
				require.False(t, errors.As(err, &failure), "a delete AWS applied is never reported as rejected")
				return
			}
			require.NoError(t, err, "AWS answers a repeat with the same resource")
		})
	}
}
