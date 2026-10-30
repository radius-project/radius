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

// Package awsfake has in-memory fakes of the AWS Cloud Control and CloudFormation clients for UCP tests. The Cloud
// Control fake keeps resources, finishes every request when its status is read, and honors client tokens, so tests can
// run a whole deploy, including retries, without AWS.
package awsfake

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
)

var _ ucp_aws.AWSCloudControlClient = (*CloudControl)(nil)
var _ ucp_aws.AWSCloudFormationClient = (*CloudFormation)(nil)

// Call is one call the Cloud Control fake received.
type Call struct {
	// Operation is GetResource, CreateResource, UpdateResource, DeleteResource, or GetResourceRequestStatus.
	Operation  string
	TypeName   string
	Identifier string
	// Document is the desired state of a create or the patch of an update.
	Document    string
	ClientToken string
}

type request struct {
	operation  cctypes.Operation
	typeName   string
	identifier string
	failure    *cctypes.ProgressEvent
}

// CloudControl is an in-memory Cloud Control. Resources are keyed by type and identifier.
type CloudControl struct {
	mu        sync.Mutex
	resources map[string]map[string]any
	requests  map[string]*request
	tokens    map[string]string
	calls     []Call
	next      int

	// Identifiers maps a type to the property that is its primary identifier. A create with that property set uses
	// its value; otherwise the fake assigns an identifier, like AWS does for a VPC.
	Identifiers map[string]string

	// WriteOnly maps a type to the paths of its write-only properties, which GetResource never returns, like AWS. "*"
	// matches every array item.
	WriteOnly map[string][]string

	// Errors maps an operation name to an error the fake returns instead of running it.
	Errors map[string]error

	// FailNext, when set, makes the next accepted create, update, or delete fail with this error code when its status
	// is read. AWS changes nothing for a failed request.
	FailNext cctypes.HandlerErrorCode
}

// NewCloudControl returns an empty CloudControl.
func NewCloudControl() *CloudControl {
	return &CloudControl{
		resources:   map[string]map[string]any{},
		requests:    map[string]*request{},
		tokens:      map[string]string{},
		Identifiers: map[string]string{},
		WriteOnly:   map[string][]string{},
		Errors:      map[string]error{},
	}
}

func key(typeName string, identifier string) string {
	return strings.ToLower(typeName) + "|" + identifier
}

// Put stores a resource, as if it was created outside Radius.
func (c *CloudControl) Put(typeName string, identifier string, properties map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resources[key(typeName, identifier)] = clone(properties)
}

// Resource returns a resource, or nil.
func (c *CloudControl) Resource(typeName string, identifier string) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return clone(c.resources[key(typeName, identifier)])
}

// Remove deletes a resource, as if it was deleted outside Radius.
func (c *CloudControl) Remove(typeName string, identifier string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.resources, key(typeName, identifier))
}

// Count returns how many resources of a type exist.
func (c *CloudControl) Count(typeName string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for k := range c.resources {
		if strings.HasPrefix(k, strings.ToLower(typeName)+"|") {
			n++
		}
	}
	return n
}

// Calls returns the calls the fake received, except status reads, and clears them.
func (c *CloudControl) Calls() []Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []Call{}
	for _, call := range c.calls {
		if call.Operation != "GetResourceRequestStatus" {
			out = append(out, call)
		}
	}
	c.calls = nil
	return out
}

func (c *CloudControl) record(call Call) error {
	c.calls = append(c.calls, call)
	return c.Errors[call.Operation]
}

func (c *CloudControl) accept(op cctypes.Operation, typeName string, identifier string) string {
	token := uuid.New().String()
	req := &request{operation: op, typeName: typeName, identifier: identifier}
	if c.FailNext != "" {
		req.failure = &cctypes.ProgressEvent{ErrorCode: c.FailNext, StatusMessage: aws.String("the request failed")}
		c.FailNext = ""
	}
	c.requests[token] = req
	return token
}

// GetResource returns a resource, or ResourceNotFoundException.
func (c *CloudControl) GetResource(ctx context.Context, params *cloudcontrol.GetResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.record(Call{Operation: "GetResource", TypeName: aws.ToString(params.TypeName), Identifier: aws.ToString(params.Identifier)}); err != nil {
		return nil, err
	}
	props, ok := c.resources[key(aws.ToString(params.TypeName), aws.ToString(params.Identifier))]
	if !ok {
		return nil, &cctypes.ResourceNotFoundException{Message: aws.String("not found")}
	}
	props = clone(props)
	for _, p := range c.WriteOnly[aws.ToString(params.TypeName)] {
		removePath(props, strings.Split(p, "/"))
	}
	b, _ := json.Marshal(props)
	return &cloudcontrol.GetResourceOutput{
		TypeName:            params.TypeName,
		ResourceDescription: &cctypes.ResourceDescription{Identifier: params.Identifier, Properties: aws.String(string(b))},
	}, nil
}

// CreateResource accepts a create. A client token it has seen returns the first request instead of creating again.
// The resource exists when the request's status is read and it succeeds.
func (c *CloudControl) CreateResource(ctx context.Context, params *cloudcontrol.CreateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CreateResourceOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	typeName := aws.ToString(params.TypeName)
	clientToken := aws.ToString(params.ClientToken)
	if err := c.record(Call{Operation: "CreateResource", TypeName: typeName, Document: aws.ToString(params.DesiredState), ClientToken: clientToken}); err != nil {
		return nil, err
	}
	if token, ok := c.tokens[clientToken]; ok && clientToken != "" {
		return &cloudcontrol.CreateResourceOutput{ProgressEvent: &cctypes.ProgressEvent{RequestToken: aws.String(token), OperationStatus: cctypes.OperationStatusInProgress}}, nil
	}

	props := map[string]any{}
	if err := json.Unmarshal([]byte(aws.ToString(params.DesiredState)), &props); err != nil {
		return nil, err
	}
	identifier, _ := props[c.Identifiers[typeName]].(string)
	if identifier == "" {
		c.next++
		identifier = "id-" + strconv.Itoa(c.next)
	}
	token := c.accept(cctypes.OperationCreate, typeName, identifier)
	if c.requests[token].failure == nil {
		c.resources[key(typeName, identifier)] = props
	}
	if clientToken != "" {
		c.tokens[clientToken] = token
	}
	return &cloudcontrol.CreateResourceOutput{ProgressEvent: &cctypes.ProgressEvent{RequestToken: aws.String(token), OperationStatus: cctypes.OperationStatusInProgress}}, nil
}

// UpdateResource applies a JSON patch to a resource.
func (c *CloudControl) UpdateResource(ctx context.Context, params *cloudcontrol.UpdateResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.UpdateResourceOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	typeName, identifier := aws.ToString(params.TypeName), aws.ToString(params.Identifier)
	if err := c.record(Call{Operation: "UpdateResource", TypeName: typeName, Identifier: identifier, Document: aws.ToString(params.PatchDocument)}); err != nil {
		return nil, err
	}
	props, ok := c.resources[key(typeName, identifier)]
	if !ok {
		return nil, &cctypes.ResourceNotFoundException{Message: aws.String("not found")}
	}
	token := c.accept(cctypes.OperationUpdate, typeName, identifier)
	if c.requests[token].failure == nil {
		ops := []map[string]any{}
		if err := json.Unmarshal([]byte(aws.ToString(params.PatchDocument)), &ops); err != nil {
			return nil, err
		}
		updated := clone(props)
		for _, op := range ops {
			if err := applyOperation(updated, op); err != nil {
				return nil, err
			}
		}
		c.resources[key(typeName, identifier)] = updated
	}
	return &cloudcontrol.UpdateResourceOutput{ProgressEvent: &cctypes.ProgressEvent{RequestToken: aws.String(token), OperationStatus: cctypes.OperationStatusInProgress}}, nil
}

// DeleteResource deletes a resource.
func (c *CloudControl) DeleteResource(ctx context.Context, params *cloudcontrol.DeleteResourceInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.DeleteResourceOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	typeName, identifier := aws.ToString(params.TypeName), aws.ToString(params.Identifier)
	if err := c.record(Call{Operation: "DeleteResource", TypeName: typeName, Identifier: identifier}); err != nil {
		return nil, err
	}
	if _, ok := c.resources[key(typeName, identifier)]; !ok {
		return nil, &cctypes.ResourceNotFoundException{Message: aws.String("not found")}
	}
	token := c.accept(cctypes.OperationDelete, typeName, identifier)
	if c.requests[token].failure == nil {
		delete(c.resources, key(typeName, identifier))
	}
	return &cloudcontrol.DeleteResourceOutput{ProgressEvent: &cctypes.ProgressEvent{RequestToken: aws.String(token), OperationStatus: cctypes.OperationStatusInProgress}}, nil
}

// GetResourceRequestStatus reports a request as finished.
func (c *CloudControl) GetResourceRequestStatus(ctx context.Context, params *cloudcontrol.GetResourceRequestStatusInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.GetResourceRequestStatusOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.record(Call{Operation: "GetResourceRequestStatus"}); err != nil {
		return nil, err
	}
	req, ok := c.requests[aws.ToString(params.RequestToken)]
	if !ok {
		return nil, &cctypes.RequestTokenNotFoundException{Message: aws.String("unknown request token")}
	}
	event := &cctypes.ProgressEvent{
		Operation:       req.operation,
		OperationStatus: cctypes.OperationStatusSuccess,
		TypeName:        aws.String(req.typeName),
		Identifier:      aws.String(req.identifier),
		RequestToken:    params.RequestToken,
	}
	if req.failure != nil {
		event.OperationStatus = cctypes.OperationStatusFailed
		event.ErrorCode = req.failure.ErrorCode
		event.StatusMessage = req.failure.StatusMessage
	}
	return &cloudcontrol.GetResourceRequestStatusOutput{ProgressEvent: event}, nil
}

// ListResources is not supported.
func (c *CloudControl) ListResources(ctx context.Context, params *cloudcontrol.ListResourcesInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourcesOutput, error) {
	return nil, fmt.Errorf("ListResources is not supported by the fake")
}

// CancelResourceRequest is not supported.
func (c *CloudControl) CancelResourceRequest(ctx context.Context, params *cloudcontrol.CancelResourceRequestInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.CancelResourceRequestOutput, error) {
	return nil, fmt.Errorf("CancelResourceRequest is not supported by the fake")
}

// ListResourceRequests is not supported.
func (c *CloudControl) ListResourceRequests(ctx context.Context, params *cloudcontrol.ListResourceRequestsInput, optFns ...func(*cloudcontrol.Options)) (*cloudcontrol.ListResourceRequestsOutput, error) {
	return nil, fmt.Errorf("ListResourceRequests is not supported by the fake")
}

// CloudFormation returns a fixed schema for each type.
type CloudFormation struct {
	// Schemas maps a type to its schema.
	Schemas map[string]string
}

// DescribeType returns the schema of a type as FULLY_MUTABLE.
func (c *CloudFormation) DescribeType(ctx context.Context, params *cloudformation.DescribeTypeInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeTypeOutput, error) {
	schema, ok := c.Schemas[aws.ToString(params.TypeName)]
	if !ok {
		return nil, fmt.Errorf("no schema for %s", aws.ToString(params.TypeName))
	}
	return &cloudformation.DescribeTypeOutput{TypeName: params.TypeName, Schema: aws.String(schema), ProvisioningType: cftypes.ProvisioningTypeFullyMutable}, nil
}

func clone(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

// applyOperation applies one add, replace, or remove JSON patch operation to obj.
func applyOperation(obj map[string]any, op map[string]any) error {
	path, _ := op["path"].(string)
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, s := range segments {
		segments[i] = strings.NewReplacer("~1", "/", "~0", "~").Replace(s)
	}
	var parent any = obj
	for _, s := range segments[:len(segments)-1] {
		switch p := parent.(type) {
		case map[string]any:
			if _, ok := p[s]; !ok {
				p[s] = map[string]any{}
			}
			parent = p[s]
		case []any:
			i, err := strconv.Atoi(s)
			if err != nil || i >= len(p) {
				return fmt.Errorf("invalid patch path %q", path)
			}
			parent = p[i]
		}
	}
	last := segments[len(segments)-1]
	switch p := parent.(type) {
	case map[string]any:
		if op["op"] == "remove" {
			delete(p, last)
		} else {
			p[last] = op["value"]
		}
	case []any:
		i, err := strconv.Atoi(last)
		if err != nil || i >= len(p) {
			return fmt.Errorf("unsupported patch path %q", path)
		}
		if op["op"] == "remove" {
			return fmt.Errorf("unsupported remove of array item %q", path)
		}
		p[i] = op["value"]
	}
	return nil
}

func removePath(obj map[string]any, segments []string) {
	if len(segments) == 1 {
		delete(obj, segments[0])
		return
	}
	if segments[1] == "*" {
		items, _ := obj[segments[0]].([]any)
		for _, item := range items {
			if m, ok := item.(map[string]any); ok && len(segments) > 2 {
				removePath(m, segments[2:])
			}
		}
		return
	}
	if m, ok := obj[segments[0]].(map[string]any); ok {
		removePath(m, segments[1:])
	}
}
