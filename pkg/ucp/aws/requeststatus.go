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

package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
)

// OperationTimeout is how long UCP waits for a Cloud Control request. A CloudFormation resource handler can run for
// up to 2160 minutes (36 hours), the maximum timeoutInMinutes of the resource schema, so UCP never gives up on a
// request that Cloud Control is still running.
const OperationTimeout = 37 * time.Hour

// The poll interval starts at MinPollInterval and doubles up to MaxPollInterval. Tests shorten them.
var (
	MinPollInterval = 2 * time.Second
	MaxPollInterval = 15 * time.Second
)

// RequestStatus is the state of a Cloud Control request.
type RequestStatus struct {
	// Done is true when the request reached a terminal state.
	Done bool

	// Succeeded is true when the request finished successfully. A delete of a resource that no longer exists counts
	// as a success.
	Succeeded bool

	// Identifier is the AWS identifier of the resource, when Cloud Control reports it.
	Identifier string

	// Code and Message describe a failure.
	Code    string
	Message string
}

// Error describes a failed request.
func (s *RequestStatus) Error() string {
	return fmt.Sprintf("AWS Cloud Control request failed: %s: %s", s.Code, s.Message)
}

// IsClientError reports whether a failed request failed because of the request, such as an invalid property value,
// rather than because of AWS.
func (s *RequestStatus) IsClientError() bool {
	switch types.HandlerErrorCode(s.Code) {
	case types.HandlerErrorCodeInvalidRequest, types.HandlerErrorCodeAlreadyExists, types.HandlerErrorCodeNotUpdatable,
		types.HandlerErrorCodeNotFound, types.HandlerErrorCodeAccessDenied, types.HandlerErrorCodeInvalidCredentials,
		types.HandlerErrorCodeResourceConflict, types.HandlerErrorCodeServiceLimitExceeded:
		return true
	default:
		return false
	}
}

// GetRequestStatus reads the state of a Cloud Control request by its request token.
func GetRequestStatus(ctx context.Context, client AWSCloudControlClient, region string, requestToken string) (*RequestStatus, error) {
	out, err := client.GetResourceRequestStatus(ctx, &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: aws.String(requestToken),
	}, func(o *cloudcontrol.Options) { o.Region = region })
	if err != nil {
		return nil, err
	}
	event := out.ProgressEvent
	if event == nil {
		return nil, fmt.Errorf("AWS Cloud Control returned no status for the request %s", requestToken)
	}

	status := &RequestStatus{Identifier: aws.ToString(event.Identifier)}
	switch event.OperationStatus {
	case types.OperationStatusSuccess:
		status.Done, status.Succeeded = true, true
	case types.OperationStatusFailed, types.OperationStatusCancelComplete:
		status.Done = true
		status.Code = string(event.ErrorCode)
		status.Message = aws.ToString(event.StatusMessage)
		if event.Operation == types.OperationDelete && event.ErrorCode == types.HandlerErrorCodeNotFound {
			status.Succeeded = true
		}
	}
	return status, nil
}

// WaitForRequest polls a Cloud Control request until it finishes or ctx ends. It only reads AWS, so running it again
// after a restart is safe. A request that finished but failed is returned as a *RequestStatus error.
func WaitForRequest(ctx context.Context, client AWSCloudControlClient, region string, requestToken string) (*RequestStatus, error) {
	interval := MinPollInterval
	for {
		status, err := GetRequestStatus(ctx, client, region, requestToken)
		if err != nil {
			return nil, err
		}
		if status.Done {
			if !status.Succeeded {
				return status, status
			}
			return status, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("stopped waiting for the AWS Cloud Control request %s: %w", requestToken, ctx.Err())
		case <-time.After(interval):
		}
		interval = min(interval*2, MaxPollInterval)
	}
}
