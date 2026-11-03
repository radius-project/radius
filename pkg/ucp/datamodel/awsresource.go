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

package datamodel

import v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"

// AWSResource is UCP's record of one AWS resource it deploys. It is stored under the resource ID that ends in the
// alias the template sets, for example /planes/aws/aws/accounts/{account}/regions/{region}/providers/AWS.EC2/VPC/app-vpc.
//
// The record is internal to UCP. A GET on the resource still reads the resource from AWS.
type AWSResource struct {
	// ID, name (the alias), type, and provisioningState. Provisioning, Updating, and Deleting mean an operation is in
	// progress. Accepted means UCP saved the record before calling AWS, and nothing is polling the operation yet.
	v1.BaseResource

	// Properties are the properties of the record.
	Properties AWSResourceProperties `json:"properties"`
}

// AWSResourceProperties are the properties of an AWS resource record.
type AWSResourceProperties struct {
	// AWSIdentifier is what Cloud Control finds the resource by, such as vpc-0a1b2c3d. It is saved before the create
	// when the properties give it, such as BucketName, and when the create finishes otherwise.
	AWSIdentifier string `json:"awsIdentifier,omitempty"`

	// Owner is who deploys this alias. A PUT or DELETE from a different owner fails.
	Owner string `json:"owner"`

	// LastApplied is the properties from the last successful deploy, without write-only values. A property in it
	// that the template no longer sets is cleared in AWS. It is nil until a deploy succeeds, including after UCP takes
	// over a resource, and it is saved without omitempty so an empty map stays distinct from nil.
	LastApplied map[string]any `json:"lastApplied"`

	// PendingLastApplied holds the new last-applied properties, without write-only values, while an operation is in
	// progress. UCP saves it before it calls AWS. It becomes LastApplied when AWS succeeds, and is dropped when AWS
	// fails, so the previous LastApplied stays.
	PendingLastApplied map[string]any `json:"pendingLastApplied"`

	// PendingOperation is the operation in progress: CREATE, UPDATE, or DELETE. The async worker sets
	// provisioningState to Updating for every operation, so the state alone cannot tell them apart.
	PendingOperation string `json:"pendingOperation,omitempty"`

	// ClientToken is a UUID that UCP generates before the first create and sends with every attempt to create the
	// resource, so Cloud Control treats a retry as the same request. It is cleared when the create succeeds, so a
	// record that has one has a first create that has not finished. A failed create removes the record.
	ClientToken string `json:"clientToken,omitempty"`

	// RequestToken is the token Cloud Control returns when it accepts a create, update, or delete. UCP polls the
	// request with it, and uses it as the operation ID.
	RequestToken string `json:"requestToken,omitempty"`

	// LastRequestToken is the request token of the last operation that finished, and LastRequestError is its error,
	// or nil when it succeeded. A redelivered operation that already finished reports this result instead of polling
	// AWS again.
	LastRequestToken string           `json:"lastRequestToken,omitempty"`
	LastRequestError *v1.ErrorDetails `json:"lastRequestError,omitempty"`
}

// ResourceTypeName returns the resource type of the record, such as AWS.EC2/VPC.
func (a *AWSResource) ResourceTypeName() string {
	return a.Type
}
