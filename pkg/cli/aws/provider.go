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

// AWSCredentialKind - AWS credential kinds supported.
type AWSCredentialKind string

const (
	// ProviderDisplayName is the text used in display for AWS.
	ProviderDisplayName        = "AWS"
	AWSCredentialKindAccessKey = "AccessKey"
	AWSCredentialKindIRSA      = "IRSA"
)

// Provider specifies the properties required to configure AWS provider for cloud resources.
type Provider struct {

	// Region is the AWS region to use.
	Region string

	// AccountID is the AWS account id.
	AccountID string

	// CredentialKind represents ucp credential kind for aws provider.
	CredentialKind AWSCredentialKind

	// AccessKey represents ucp credential kind for aws access key credentials.
	AccessKey *AccessKeyCredential

	// IRSA represents ucp credential kind for aws irsa credentials.
	IRSA *IRSACredential
}

type AccessKeyCredential struct {
	// AccessKeyID is the access key id for the AWS account.
	AccessKeyID string

	// SecretAccessKey is the secret access key for the AWS account.
	SecretAccessKey string
}

type IRSACredential struct {
	// RoleARN for AWS IRSA identity
	RoleARN string
}
