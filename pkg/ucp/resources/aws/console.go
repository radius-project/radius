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
	"net/url"
	"regexp"
	"strings"

	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"

	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	commercialConsoleHostFormat  = "%s.console.aws.amazon.com"
	commercialGlobalConsoleHost  = "console.aws.amazon.com"
	govCloudConsoleHost          = "console.amazonaws-us-gov.com"
	chinaConsoleHost             = "console.amazonaws.cn"
	europeanSovereignConsoleHost = "console.amazonaws-eusc.eu"

	// resourcePageURLPrefix is the commercial console redirect that opens the page of the resource an ARN names.
	resourcePageURLPrefix = "https://console.aws.amazon.com/go/view?arn="

	// commercialPartition is the ARN partition of the commercial AWS regions.
	commercialPartition = "aws"

	// globalRegion is the region Radius uses for AWS resources whose ARN has no region, such as S3 buckets.
	globalRegion = "global"
)

// globalConsoleHosts maps an AWS partition, which is the name of the AWS plane, to its console host for services that
// are global in the console.
var globalConsoleHosts = map[string]string{
	"aws":        commercialGlobalConsoleHost,
	"aws-us-gov": govCloudConsoleHost,
	"aws-cn":     chinaConsoleHost,
	"aws-eusc":   europeanSovereignConsoleHost,
}

// regionConsoles maps the region names of each AWS partition to the console host for that partition. The patterns
// follow the partition region patterns of the AWS SDKs. The ISO partitions are left out because their consoles are only
// reachable from private networks.
//
// The first matching entry wins, so the commercial entry must stay last: China regions such as cn-north-1 also match
// the commercial pattern and would otherwise link to the commercial console.
var regionConsoles = []struct {
	region *regexp.Regexp
	host   string

	// regional is true when host is a format string that takes the region, because the console has a host per region.
	regional bool
}{
	{region: regexp.MustCompile(`^us-gov-[a-z]+-\d+$`), host: govCloudConsoleHost},
	{region: regexp.MustCompile(`^cn-[a-z]+-\d+$`), host: chinaConsoleHost},
	{region: regexp.MustCompile(`^eusc-[a-z]+-[a-z]+-\d+$`), host: europeanSovereignConsoleHost},
	{region: regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d+$`), host: commercialConsoleHostFormat, regional: true},
}

type consolePage struct {
	path string
	hash string

	// global is true when the console page is not tied to a region, so it can be linked for a resource in the global
	// region.
	global bool
}

var (
	amazonMQBrokers         = consolePage{path: "/amazon-mq/home", hash: "#/brokers"}
	appSyncAPIs             = consolePage{path: "/appsync/home", hash: "#/apis"}
	bedrockAgents           = consolePage{path: "/bedrock/home", hash: "#/agents"}
	bedrockGuardrails       = consolePage{path: "/bedrock/home", hash: "#/guardrails"}
	bedrockKnowledgeBases   = consolePage{path: "/bedrock/home", hash: "#/knowledge-bases"}
	cloudWatchLogGroups     = consolePage{path: "/cloudwatch/home", hash: "#logsV2:log-groups"}
	codeBuildProjects       = consolePage{path: "/codesuite/codebuild/projects"}
	daxClusters             = consolePage{path: "/dynamodbv2/home", hash: "#dax-clusters"}
	dmsReplicationInstances = consolePage{path: "/dms/v2/home", hash: "#replicationInstances"}
	docDBClusters           = consolePage{path: "/docdb/home", hash: "#clusters"}
	dynamoDBTables          = consolePage{path: "/dynamodbv2/home", hash: "#tables"}
	ec2LoadBalancers        = consolePage{path: "/ec2/home", hash: "#LoadBalancers:"}
	ec2SecurityGroups       = consolePage{path: "/ec2/home", hash: "#SecurityGroups:"}
	ec2Snapshots            = consolePage{path: "/ec2/home", hash: "#Snapshots:"}
	ec2TargetGroups         = consolePage{path: "/ec2/home", hash: "#TargetGroups:"}
	ec2Volumes              = consolePage{path: "/ec2/home", hash: "#Volumes:"}
	ecrRepositories         = consolePage{path: "/ecr/private-registry/repositories"}
	eksClusters             = consolePage{path: "/eks/home", hash: "#/clusters"}
	// ElastiCache links to its dashboard because the ID does not name the engine, and each engine has its own list.
	elastiCacheClusters   = consolePage{path: "/elasticache/home"}
	emrClusters           = consolePage{path: "/emr/home", hash: "#/clusters"}
	fsxFileSystems        = consolePage{path: "/fsx/home", hash: "#file-systems"}
	iamPolicies           = consolePage{path: "/iam/home", hash: "#/policies", global: true}
	iamRoles              = consolePage{path: "/iam/home", hash: "#/roles", global: true}
	iamUsers              = consolePage{path: "/iam/home", hash: "#/users", global: true}
	lambdaFunctions       = consolePage{path: "/lambda/home", hash: "#/functions"}
	mediaLiveChannels     = consolePage{path: "/medialive/home", hash: "#/channels"}
	memoryDBClusters      = consolePage{path: "/memorydb/home", hash: "#/clusters"}
	mskClusters           = consolePage{path: "/msk/home", hash: "#/clusters"}
	openSearchDomains     = consolePage{path: "/aos/home", hash: "#opensearch/domains"}
	rdsDatabases          = consolePage{path: "/rds/home", hash: "#databases:"}
	rdsSubnetGroups       = consolePage{path: "/rds/home", hash: "#db-subnet-groups-list:"}
	s3Buckets             = consolePage{path: "/s3/buckets", global: true}
	secretsManagerSecrets = consolePage{path: "/secretsmanager/listsecrets"}
	snsTopics             = consolePage{path: "/sns/v3/home", hash: "#/topics"}
	sqsQueues             = consolePage{path: "/sqs/v3/home", hash: "#/queues"}
	vpcClientVPNEndpoints = consolePage{path: "/vpc/home", hash: "#ClientVPNEndpoints:"}
)

// consolePages maps each known AWS resource type, keyed by its lower-cased UCP type, to the console list page of its
// service. A list page is the fallback for resources without a resource page link, such as resources with no ARN,
// because AWS has no documented link format to a single resource that works for every type. The region is a query parameter, so the hash, when present, follows it.
//
// AWS.* types are deployed through UCP. Terraform.AWS/* types are discovered from Terraform recipe state, which records
// only resources with an arn attribute, so Terraform types without one are not listed.
var consolePages = map[string]consolePage{
	"aws.amazonmq/broker":                       amazonMQBrokers,
	"aws.appsync/graphqlschema":                 appSyncAPIs,
	"aws.bedrock/agent":                         bedrockAgents,
	"aws.bedrock/guardrail":                     bedrockGuardrails,
	"aws.bedrock/knowledgebase":                 bedrockKnowledgeBases,
	"aws.codebuild/project":                     codeBuildProjects,
	"aws.dax/cluster":                           daxClusters,
	"aws.dms/replicationinstance":               dmsReplicationInstances,
	"aws.docdb/dbcluster":                       docDBClusters,
	"aws.docdb/dbinstance":                      docDBClusters,
	"aws.docdbelastic/cluster":                  docDBClusters,
	"aws.dynamodb/globaltable":                  dynamoDBTables,
	"aws.dynamodb/table":                        dynamoDBTables,
	"aws.ec2/clientvpnendpoint":                 vpcClientVPNEndpoints,
	"aws.ec2/clientvpntargetnetworkassociation": vpcClientVPNEndpoints,
	"aws.ec2/securitygroup":                     ec2SecurityGroups,
	"aws.ecr/repository":                        ecrRepositories,
	"aws.eks/cluster":                           eksClusters,
	"aws.elasticache/cachecluster":              elastiCacheClusters,
	"aws.elasticache/replicationgroup":          elastiCacheClusters,
	"aws.elasticache/serverlesscache":           elastiCacheClusters,
	"aws.emr/cluster":                           emrClusters,
	"aws.fsx/filesystem":                        fsxFileSystems,
	"aws.iam/accesskey":                         iamUsers,
	// The IAM console has no instance profile list. Instance profiles are shown on the role they hold.
	"aws.iam/instanceprofile":      iamRoles,
	"aws.iam/managedpolicy":        iamPolicies,
	"aws.iam/role":                 iamRoles,
	"aws.lambda/function":          lambdaFunctions,
	"aws.medialive/channel":        mediaLiveChannels,
	"aws.logs/loggroup":            cloudWatchLogGroups,
	"aws.memorydb/cluster":         memoryDBClusters,
	"aws.msk/cluster":              mskClusters,
	"aws.msk/serverlesscluster":    mskClusters,
	"aws.opensearchservice/domain": openSearchDomains,
	"aws.rds/dbcluster":            rdsDatabases,
	"aws.rds/dbinstance":           rdsDatabases,
	"aws.s3/bucket":                s3Buckets,
	"aws.secretsmanager/secret":    secretsManagerSecrets,
	"aws.sns/topic":                snsTopics,
	"aws.sqs/queue":                sqsQueues,

	"terraform.aws/aws_alb":                           ec2LoadBalancers,
	"terraform.aws/aws_alb_listener":                  ec2LoadBalancers,
	"terraform.aws/aws_alb_target_group":              ec2TargetGroups,
	"terraform.aws/aws_appsync_graphql_api":           appSyncAPIs,
	"terraform.aws/aws_bedrockagent_knowledge_base":   bedrockKnowledgeBases,
	"terraform.aws/aws_cloudwatch_log_group":          cloudWatchLogGroups,
	"terraform.aws/aws_codebuild_project":             codeBuildProjects,
	"terraform.aws/aws_dax_cluster":                   daxClusters,
	"terraform.aws/aws_db_instance":                   rdsDatabases,
	"terraform.aws/aws_db_subnet_group":               rdsSubnetGroups,
	"terraform.aws/aws_docdb_cluster":                 docDBClusters,
	"terraform.aws/aws_docdb_cluster_instance":        docDBClusters,
	"terraform.aws/aws_docdbelastic_cluster":          docDBClusters,
	"terraform.aws/aws_dynamodb_global_table":         dynamoDBTables,
	"terraform.aws/aws_dynamodb_table":                dynamoDBTables,
	"terraform.aws/aws_ebs_snapshot":                  ec2Snapshots,
	"terraform.aws/aws_ebs_volume":                    ec2Volumes,
	"terraform.aws/aws_ec2_client_vpn_endpoint":       vpcClientVPNEndpoints,
	"terraform.aws/aws_ecr_repository":                ecrRepositories,
	"terraform.aws/aws_eks_cluster":                   eksClusters,
	"terraform.aws/aws_elasticache_cluster":           elastiCacheClusters,
	"terraform.aws/aws_elasticache_replication_group": elastiCacheClusters,
	"terraform.aws/aws_elasticache_serverless_cache":  elastiCacheClusters,
	"terraform.aws/aws_emr_cluster":                   emrClusters,
	"terraform.aws/aws_fsx_lustre_file_system":        fsxFileSystems,
	"terraform.aws/aws_fsx_ontap_file_system":         fsxFileSystems,
	"terraform.aws/aws_fsx_openzfs_file_system":       fsxFileSystems,
	"terraform.aws/aws_fsx_windows_file_system":       fsxFileSystems,
	"terraform.aws/aws_iam_instance_profile":          iamRoles,
	"terraform.aws/aws_iam_policy":                    iamPolicies,
	"terraform.aws/aws_iam_role":                      iamRoles,
	"terraform.aws/aws_lambda_function":               lambdaFunctions,
	"terraform.aws/aws_lb":                            ec2LoadBalancers,
	"terraform.aws/aws_lb_listener":                   ec2LoadBalancers,
	"terraform.aws/aws_lb_target_group":               ec2TargetGroups,
	"terraform.aws/aws_medialive_channel":             mediaLiveChannels,
	"terraform.aws/aws_memorydb_cluster":              memoryDBClusters,
	"terraform.aws/aws_mq_broker":                     amazonMQBrokers,
	"terraform.aws/aws_msk_cluster":                   mskClusters,
	"terraform.aws/aws_msk_serverless_cluster":        mskClusters,
	"terraform.aws/aws_opensearch_domain":             openSearchDomains,
	"terraform.aws/aws_rds_cluster":                   rdsDatabases,
	"terraform.aws/aws_rds_cluster_instance":          rdsDatabases,
	"terraform.aws/aws_s3_bucket":                     s3Buckets,
	"terraform.aws/aws_secretsmanager_secret":         secretsManagerSecrets,
	"terraform.aws/aws_secretsmanager_secret_version": secretsManagerSecrets,
	"terraform.aws/aws_security_group":                ec2SecurityGroups,
	"terraform.aws/aws_sns_topic":                     snsTopics,
	"terraform.aws/aws_sqs_queue":                     sqsQueues,
}

// resourcePageDenyList holds AWS resource types, keyed by their lower-cased UCP type, whose resource page link is
// known not to work. They fall back to their service's list page, as resources with no ARN do. AWS does not document the console's go/view
// redirect, so a type that turns out to be broken is added here.
var resourcePageDenyList = map[string]struct{}{}

// ConsoleURL returns a link to the AWS console for the resource id, or "" when there is none, so a caller never shows
// a made-up link. arn is the resource's ARN, or "" when it is not known.
//
// When arn is an ARN in the commercial partition and the type is not in resourcePageDenyList, the link opens the
// resource's own page. The console's go/view redirect is not known to work in GovCloud, China, or European Sovereign
// Cloud, so those resources keep their list page. Otherwise the link opens the list page of the service that owns the
// resource, in the region the ID names. GovCloud, China, and European Sovereign Cloud regions use the console of their
// partition. A resource in the global region gets a list page link only when that page is global, such as S3 buckets,
// using the console of the partition the AWS plane names. There is no list page link when the type has no known page,
// or the region is missing or not a valid region name.
func ConsoleURL(id resources.ID, arn string) string {
	if !IsAWSResource(id) {
		return ""
	}

	resourceType := strings.ToLower(id.Type())
	if link := resourcePageURL(resourceType, arn); link != "" {
		return link
	}

	page, ok := consolePages[resourceType]
	if !ok {
		return ""
	}

	region := id.FindScope(ScopeRegions)
	if region == globalRegion {
		host, ok := globalConsoleHosts[strings.ToLower(id.ScopeSegments()[0].Name)]
		if !page.global || !ok {
			return ""
		}
		return fmt.Sprintf("https://%s%s%s", host, page.path, page.hash)
	}

	for _, console := range regionConsoles {
		if !console.region.MatchString(region) {
			continue
		}
		host := console.host
		if console.regional {
			host = fmt.Sprintf(console.host, region)
		}
		return fmt.Sprintf("https://%s%s?region=%s%s", host, page.path, region, page.hash)
	}

	return ""
}

// resourcePageURL returns a link to the console page of the resource arn names, or "" when arn is not an ARN in the
// commercial partition, or resourceType is in resourcePageDenyList.
func resourcePageURL(resourceType string, arn string) string {
	if _, denied := resourcePageDenyList[resourceType]; denied {
		return ""
	}

	parsed, err := awsarn.Parse(arn)
	if err != nil || parsed.Partition != commercialPartition {
		return ""
	}

	return resourcePageURLPrefix + url.QueryEscape(arn)
}
