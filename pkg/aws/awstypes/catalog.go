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

// Package awstypes lists the AWS resource types that Radius cannot manage through AWS Cloud Control and that must be
// known without calling AWS:
//
//   - legacy types, which have a supported replacement
//   - deprecated types, from services AWS has ended and from CloudFormation template features
//   - read-only types, which AWS creates on its own (generated from the CloudFormation registry)
//
// UCP rejects legacy and deprecated types before it calls AWS. For every other type, UCP reads the provisioning type
// from CloudFormation at run time, so a type starts working as soon as AWS adds Cloud Control support for it. The
// read-only list is for callers with no AWS access, such as the recipe engine.
package awstypes

//go:generate go run ./gen

import (
	"fmt"
	"slices"
	"strings"
)

// legacyTypes maps each legacy type to the type, or property, that replaces it. Cloud Control manages every
// replacement.
var legacyTypes = map[string]string{
	"AWS::AmazonMQ::ConfigurationAssociation": "the Configuration property of AWS::AmazonMQ::Broker",
	"AWS::Elasticsearch::Domain":              "AWS::OpenSearchService::Domain",
	"AWS::IAM::Policy":                        "AWS::IAM::ManagedPolicy, or AWS::IAM::RolePolicy, AWS::IAM::UserPolicy, or AWS::IAM::GroupPolicy for an inline policy",
	"AWS::IAM::UserToGroupAddition":           "the Groups property of AWS::IAM::User",
	"AWS::Route53::RecordSetGroup":            "AWS::Route53::RecordSet",
	"AWS::SNS::TopicPolicy":                   "AWS::SNS::TopicInlinePolicy",
	"AWS::SQS::QueuePolicy":                   "AWS::SQS::QueueInlinePolicy",
}

// deprecatedTypes are the deprecated types, grouped by the reason shown to the user. Types are listed one by one,
// rather than by service, because some services that AWS ended still have types that Cloud Control manages.
var deprecatedTypes = []struct {
	reason string
	types  []string
}{
	{
		reason: "AWS App Mesh reached end of support",
		types: []string{
			"AWS::AppMesh::GatewayRoute",
			"AWS::AppMesh::Mesh",
			"AWS::AppMesh::Route",
			"AWS::AppMesh::VirtualGateway",
			"AWS::AppMesh::VirtualNode",
			"AWS::AppMesh::VirtualRouter",
			"AWS::AppMesh::VirtualService",
		},
	},
	{
		reason: "it is a CloudFormation template feature, not an AWS resource",
		types: []string{
			"AWS::CloudFormation::CustomResource",
			"AWS::CloudFormation::Macro",
			"AWS::CloudFormation::WaitCondition",
			"AWS::CloudFormation::WaitConditionHandle",
		},
	},
	{
		reason: "AWS CodeStar was discontinued",
		types: []string{
			"AWS::CodeStar::GitHubRepository",
		},
	},
	{
		reason: "AWS IoT Greengrass V1 no longer accepts new customers. Use the AWS::GreengrassV2 types",
		types: []string{
			"AWS::Greengrass::ConnectorDefinition",
			"AWS::Greengrass::ConnectorDefinitionVersion",
			"AWS::Greengrass::CoreDefinition",
			"AWS::Greengrass::CoreDefinitionVersion",
			"AWS::Greengrass::DeviceDefinition",
			"AWS::Greengrass::DeviceDefinitionVersion",
			"AWS::Greengrass::FunctionDefinition",
			"AWS::Greengrass::FunctionDefinitionVersion",
			"AWS::Greengrass::Group",
			"AWS::Greengrass::GroupVersion",
			"AWS::Greengrass::LoggerDefinition",
			"AWS::Greengrass::LoggerDefinitionVersion",
			"AWS::Greengrass::ResourceDefinition",
			"AWS::Greengrass::ResourceDefinitionVersion",
			"AWS::Greengrass::SubscriptionDefinition",
			"AWS::Greengrass::SubscriptionDefinitionVersion",
		},
	},
	{
		reason: "AWS IoT Things Graph was discontinued",
		types: []string{
			"AWS::IoTThingsGraph::FlowTemplate",
		},
	},
	{
		reason: "Kinesis Data Analytics for SQL applications was discontinued. Use AWS::KinesisAnalyticsV2::Application",
		types: []string{
			"AWS::KinesisAnalytics::Application",
			"AWS::KinesisAnalytics::ApplicationOutput",
			"AWS::KinesisAnalytics::ApplicationReferenceDataSource",
		},
	},
	{
		reason: "AWS Elemental MediaStore reached end of support",
		types: []string{
			"AWS::MediaStore::Container",
		},
	},
	{
		reason: "AWS OpsWorks reached end of life",
		types: []string{
			"AWS::OpsWorks::App",
			"AWS::OpsWorks::ElasticLoadBalancerAttachment",
			"AWS::OpsWorks::Instance",
			"AWS::OpsWorks::Layer",
			"AWS::OpsWorks::Stack",
			"AWS::OpsWorks::UserProfile",
			"AWS::OpsWorks::Volume",
		},
	},
	{
		reason: "the Amazon Pinpoint email API is replaced by the Amazon SES API v2. Use the AWS::SES types",
		types: []string{
			"AWS::PinpointEmail::ConfigurationSet",
			"AWS::PinpointEmail::ConfigurationSetEventDestination",
			"AWS::PinpointEmail::DedicatedIpPool",
			"AWS::PinpointEmail::Identity",
		},
	},
	{
		reason: "Amazon Pinpoint reached end of support",
		types: []string{
			"AWS::Pinpoint::ADMChannel",
			"AWS::Pinpoint::APNSChannel",
			"AWS::Pinpoint::APNSSandboxChannel",
			"AWS::Pinpoint::APNSVoipChannel",
			"AWS::Pinpoint::APNSVoipSandboxChannel",
			"AWS::Pinpoint::App",
			"AWS::Pinpoint::ApplicationSettings",
			"AWS::Pinpoint::BaiduChannel",
			"AWS::Pinpoint::Campaign",
			"AWS::Pinpoint::EmailChannel",
			"AWS::Pinpoint::EmailTemplate",
			"AWS::Pinpoint::EventStream",
			"AWS::Pinpoint::GCMChannel",
			"AWS::Pinpoint::PushTemplate",
			"AWS::Pinpoint::SMSChannel",
			"AWS::Pinpoint::Segment",
			"AWS::Pinpoint::SmsTemplate",
			"AWS::Pinpoint::VoiceChannel",
		},
	},
	{
		reason: "Amazon QLDB reached end of support",
		types: []string{
			"AWS::QLDB::Ledger",
		},
	},
	{
		reason: "AWS WAF Classic reached end of support. Use the AWS::WAFv2 types",
		types: []string{
			"AWS::WAF::ByteMatchSet",
			"AWS::WAF::IPSet",
			"AWS::WAF::Rule",
			"AWS::WAF::SizeConstraintSet",
			"AWS::WAF::SqlInjectionMatchSet",
			"AWS::WAF::WebACL",
			"AWS::WAF::XssMatchSet",
			"AWS::WAFRegional::ByteMatchSet",
			"AWS::WAFRegional::GeoMatchSet",
			"AWS::WAFRegional::IPSet",
			"AWS::WAFRegional::RateBasedRule",
			"AWS::WAFRegional::RegexPatternSet",
			"AWS::WAFRegional::Rule",
			"AWS::WAFRegional::SizeConstraintSet",
			"AWS::WAFRegional::SqlInjectionMatchSet",
			"AWS::WAFRegional::WebACL",
			"AWS::WAFRegional::WebACLAssociation",
			"AWS::WAFRegional::XssMatchSet",
		},
	},
	{
		reason: "CloudFront RTMP distributions were discontinued",
		types: []string{
			"AWS::CloudFront::StreamingDistribution",
		},
	},
	{
		reason: "it is for EC2-Classic, which AWS retired",
		types: []string{
			"AWS::ElastiCache::SecurityGroup",
			"AWS::ElastiCache::SecurityGroupIngress",
			"AWS::RDS::DBSecurityGroup",
			"AWS::RDS::DBSecurityGroupIngress",
			"AWS::Redshift::ClusterSecurityGroup",
			"AWS::Redshift::ClusterSecurityGroupIngress",
		},
	},
	{
		reason: "AWS Glue development endpoints are deprecated. Use AWS Glue interactive sessions",
		types: []string{
			"AWS::Glue::DevEndpoint",
		},
	},
}

// Replacement returns what replaces a legacy type, and whether typeName is a legacy type. The match ignores case.
func Replacement(typeName string) (string, bool) {
	for k, v := range legacyTypes {
		if strings.EqualFold(k, typeName) {
			return v, true
		}
	}
	return "", false
}

// DeprecatedReason returns why Radius does not support a deprecated type, and whether typeName is deprecated. The
// match ignores case.
func DeprecatedReason(typeName string) (string, bool) {
	for _, group := range deprecatedTypes {
		if slices.ContainsFunc(group.types, func(t string) bool { return strings.EqualFold(t, typeName) }) {
			return group.reason, true
		}
	}
	return "", false
}

// UnsupportedMessage returns the error message for a legacy or deprecated type, and false for any other type.
func UnsupportedMessage(typeName string) (string, bool) {
	if replacement, ok := Replacement(typeName); ok {
		return fmt.Sprintf("Radius does not support the legacy AWS resource type %s. Use %s instead.", typeName, replacement), true
	}
	if reason, ok := DeprecatedReason(typeName); ok {
		return fmt.Sprintf("Radius does not support the AWS resource type %s: %s.", typeName, reason), true
	}
	return "", false
}

// IsReadOnly reports whether typeName is a type that AWS creates on its own. Radius can reference these types with
// `existing` but never creates or manages them. The match ignores case.
func IsReadOnly(typeName string) bool {
	return slices.ContainsFunc(readOnlyTypes, func(t string) bool { return strings.EqualFold(t, typeName) })
}

// ReadOnlyTypes returns the read-only types, sorted.
func ReadOnlyTypes() []string {
	return slices.Clone(readOnlyTypes)
}

// ReadOnlyMessage returns the error message for a create of, or a change to, a read-only type.
func ReadOnlyMessage(typeName string) string {
	return fmt.Sprintf("The AWS resource type %s is created by AWS, so Radius cannot create or change it. Reference an existing resource with the Bicep `existing` keyword instead.", typeName)
}
