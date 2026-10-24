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
	"fmt"
	"regexp"
	"strings"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

// consoleRegion matches commercial and GovCloud region names such as us-west-2 and us-gov-west-1. China regions match
// the pattern too but are excluded by ConsoleURL because they use a different console.
var consoleRegion = regexp.MustCompile(`^[a-z]{2}(-gov)?-[a-z]+-\d$`)

const (
	commercialConsoleHostFormat = "%s.console.aws.amazon.com"
	govCloudConsoleHost         = "console.amazonaws-us-gov.com"
)

type consolePage struct {
	path string
	hash string
}

// consolePages maps each known AWS resource type, keyed by its lower-cased UCP type, to the console list page of its
// service. A list page is used because AWS has no documented link format to a single resource that works for every
// type. The region is a query parameter, so the hash, when present, follows it.
var consolePages = map[string]consolePage{
	"aws.amazonmq/broker":                       {path: "/amazon-mq/home", hash: "#/brokers"},
	"aws.appsync/graphqlschema":                 {path: "/appsync/home", hash: "#/apis"},
	"aws.bedrock/agent":                         {path: "/bedrock/home", hash: "#/agents"},
	"aws.bedrock/guardrail":                     {path: "/bedrock/home", hash: "#/guardrails"},
	"aws.bedrock/knowledgebase":                 {path: "/bedrock/home", hash: "#/knowledge-bases"},
	"aws.codebuild/project":                     {path: "/codesuite/codebuild/projects"},
	"aws.dax/cluster":                           {path: "/dynamodbv2/home", hash: "#dax-clusters"},
	"aws.dms/replicationinstance":               {path: "/dms/v2/home", hash: "#replicationInstances"},
	"aws.docdb/dbcluster":                       {path: "/docdb/home", hash: "#clusters"},
	"aws.docdb/dbinstance":                      {path: "/docdb/home", hash: "#clusters"},
	"aws.docdbelastic/cluster":                  {path: "/docdb/home", hash: "#clusters"},
	"aws.dynamodb/globaltable":                  {path: "/dynamodbv2/home", hash: "#tables"},
	"aws.dynamodb/table":                        {path: "/dynamodbv2/home", hash: "#tables"},
	"aws.ec2/clientvpnendpoint":                 {path: "/vpc/home", hash: "#ClientVPNEndpoints:"},
	"aws.ec2/clientvpntargetnetworkassociation": {path: "/vpc/home", hash: "#ClientVPNEndpoints:"},
	"aws.ec2/securitygroup":                     {path: "/ec2/home", hash: "#SecurityGroups:"},
	"aws.ecr/repository":                        {path: "/ecr/private-registry/repositories"},
	"aws.eks/cluster":                           {path: "/eks/home", hash: "#/clusters"},
	"aws.elasticache/cachecluster":              {path: "/elasticache/home", hash: "#/redis"},
	"aws.elasticache/replicationgroup":          {path: "/elasticache/home", hash: "#/redis"},
	"aws.elasticache/serverlesscache":           {path: "/elasticache/home", hash: "#/redis"},
	"aws.emr/cluster":                           {path: "/emr/home", hash: "#/clusters"},
	"aws.fsx/filesystem":                        {path: "/fsx/home", hash: "#file-systems"},
	"aws.iam/accesskey":                         {path: "/iam/home", hash: "#/users"},
	"aws.lambda/function":                       {path: "/lambda/home", hash: "#/functions"},
	"aws.medialive/channel":                     {path: "/medialive/home", hash: "#/channels"},
	"aws.memorydb/cluster":                      {path: "/memorydb/home", hash: "#/clusters"},
	"aws.msk/cluster":                           {path: "/msk/home", hash: "#/clusters"},
	"aws.msk/serverlesscluster":                 {path: "/msk/home", hash: "#/clusters"},
	"aws.opensearchservice/domain":              {path: "/aos/home", hash: "#opensearch/domains"},
	"aws.rds/dbcluster":                         {path: "/rds/home", hash: "#databases:"},
	"aws.rds/dbinstance":                        {path: "/rds/home", hash: "#databases:"},
	"aws.s3/bucket":                             {path: "/s3/buckets"},
	"aws.secretsmanager/secret":                 {path: "/secretsmanager/listsecrets"},
	"aws.sns/topic":                             {path: "/sns/v3/home", hash: "#/topics"},
	"aws.sqs/queue":                             {path: "/sqs/v3/home", hash: "#/queues"},
}

// ConsoleURL returns a link to the AWS console list page of the service that owns the resource id, in the region the
// ID names. GovCloud regions use the GovCloud console. It returns "" when id is not an AWS resource, its type has no
// known page, or its region is missing or not a commercial or GovCloud region, so a caller never shows a made-up link.
func ConsoleURL(id resources.ID) string {
	if !IsAWSResource(id) {
		return ""
	}

	region := id.FindScope(ScopeRegions)
	if !consoleRegion.MatchString(region) || strings.HasPrefix(region, "cn-") {
		return ""
	}

	page, ok := consolePages[strings.ToLower(id.Type())]
	if !ok {
		return ""
	}

	host := fmt.Sprintf(commercialConsoleHostFormat, region)
	if strings.Contains(region, "-gov-") {
		host = govCloudConsoleHost
	}

	return fmt.Sprintf("https://%s%s?region=%s%s", host, page.path, region, page.hash)
}
