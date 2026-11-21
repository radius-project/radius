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
	"context"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// partnerAccount is the account of the fake's clusters, and partnerCluster one of them.
	partnerAccount = "123456789012"
	partnerCluster = "analytics"

	// partnerClusterType keeps the fake's clusters in the store, apart from the integrations the handler serves.
	partnerClusterType = "AWS::Redshift::Cluster"

	// partnerValidation is the code of a value outside the limits of the API reference.
	partnerValidation = "ValidationError"
)

// partnerClusterPattern is the form of a cluster identifier the API reference gives, in any case.
var partnerClusterPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,63}$`)

// partnerNames are the partners the fake supports: ones AWS added in a run against real AWS. AWS answered Sisu, and
// a name in another case, such as fivetran, with PartnerNotFound.
var partnerNames = []string{"Datacoral", "Etleap", "Fivetran", "Informatica", "Matillion"}

// partnerFault returns the client fault AWS returns with the code.
func partnerFault(code string, message string) error {
	return &smithy.GenericAPIError{Code: code, Message: message, Fault: smithy.FaultClient}
}

// addPartnerClusters adds the fake's clusters to store.
func addPartnerClusters(store *awsfake.CloudControl) {
	for _, name := range []string{partnerCluster, "reports"} {
		store.Put(partnerClusterType, name, map[string]any{"ClusterIdentifier": name})
	}
}

// partnerFake is an in-memory Redshift API for partner integrations. It keeps them in an awsfake store, in the
// type's property format, so the suite's Cloud Control sees the same resources, and the clusters under
// partnerClusterType. It follows the rules in partnerAPI, and where AWS's docs are silent it takes the side that is
// harder for the handler: a deleted integration is still listed for one read.
type partnerFake struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// deleted holds deleted integrations still listed for one read, by identifier.
	deleted map[string]map[string]any

	// errs maps a method name to an error the call returns instead of acting, and calls counts the calls of each.
	errs  map[string]error
	calls map[string]int
}

var _ PartnerAPI = (*partnerFake)(nil)

func newPartnerFake(store *awsfake.CloudControl) *partnerFake {
	return &partnerFake{store: store, deleted: map[string]map[string]any{}, errs: map[string]error{}, calls: map[string]int{}}
}

func (f *partnerFake) begin(method string) error {
	f.calls[method]++
	return f.errs[method]
}

// check checks the values of a call against the API reference's limits, and that the account has the cluster. name
// is nil for a DescribePartners call that leaves the database or partner out.
func (f *partnerFake) check(account *string, cluster *string, database *string, name *string) error {
	values := map[string]*string{"AccountId": account, "ClusterIdentifier": cluster, "DatabaseName": database, "PartnerName": name}
	for _, field := range partnerFields {
		value := values[field.name]
		if value == nil {
			if field.name == "AccountId" || field.name == "ClusterIdentifier" {
				return partnerFault(partnerValidation, field.name+" is required")
			}
			continue
		}
		pattern := field.pattern
		if field.name == "ClusterIdentifier" {
			// AWS takes a cluster identifier in any case.
			pattern = partnerClusterPattern
		}
		if !pattern.MatchString(*value) {
			return partnerFault(partnerValidation, "Value at '"+field.name+"' failed to satisfy constraint")
		}
	}
	if aws.ToString(account) != partnerAccount || f.store.Resource(partnerClusterType, strings.ToLower(aws.ToString(cluster))) == nil {
		return partnerFault(partnerClusterNotFound, "Cluster not found: "+aws.ToString(cluster))
	}
	return nil
}

// partnerKey returns the identifier of an integration, with the cluster in lowercase, as AWS keeps it.
func partnerKey(account *string, cluster *string, database *string, name *string) string {
	return strings.Join([]string{aws.ToString(account), strings.ToLower(aws.ToString(cluster)), aws.ToString(database), aws.ToString(name)}, "|")
}

func (f *partnerFake) AddPartner(ctx context.Context, in *redshift.AddPartnerInput, _ ...func(*redshift.Options)) (*redshift.AddPartnerOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("AddPartner"); err != nil {
		return nil, err
	}
	if in.DatabaseName == nil || in.PartnerName == nil {
		return nil, partnerFault(partnerValidation, "DatabaseName and PartnerName are required")
	}
	if err := f.check(in.AccountId, in.ClusterIdentifier, in.DatabaseName, in.PartnerName); err != nil {
		return nil, err
	}
	if !slices.Contains(partnerNames, aws.ToString(in.PartnerName)) {
		return nil, partnerFault(partnerNotFound, "Partner not supported.")
	}
	key := partnerKey(in.AccountId, in.ClusterIdentifier, in.DatabaseName, in.PartnerName)
	if f.store.Resource(partnerType, key) == nil {
		f.store.Put(partnerType, key, map[string]any{
			"AccountId":         aws.ToString(in.AccountId),
			"ClusterIdentifier": strings.ToLower(aws.ToString(in.ClusterIdentifier)),
			"DatabaseName":      aws.ToString(in.DatabaseName),
			"PartnerName":       aws.ToString(in.PartnerName),
		})
	}
	delete(f.deleted, key)
	return &redshift.AddPartnerOutput{DatabaseName: in.DatabaseName, PartnerName: in.PartnerName}, nil
}

func (f *partnerFake) DeletePartner(ctx context.Context, in *redshift.DeletePartnerInput, _ ...func(*redshift.Options)) (*redshift.DeletePartnerOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeletePartner"); err != nil {
		return nil, err
	}
	if in.DatabaseName == nil || in.PartnerName == nil {
		return nil, partnerFault(partnerValidation, "DatabaseName and PartnerName are required")
	}
	if err := f.check(in.AccountId, in.ClusterIdentifier, in.DatabaseName, in.PartnerName); err != nil {
		return nil, err
	}
	key := partnerKey(in.AccountId, in.ClusterIdentifier, in.DatabaseName, in.PartnerName)
	props := f.store.Resource(partnerType, key)
	if props == nil {
		return nil, partnerFault(partnerNotFound, "Partner integration not found. Nothing to delete.")
	}
	f.store.Remove(partnerType, key)
	f.deleted[key] = props
	return &redshift.DeletePartnerOutput{DatabaseName: in.DatabaseName, PartnerName: in.PartnerName}, nil
}

func (f *partnerFake) DescribePartners(ctx context.Context, in *redshift.DescribePartnersInput, _ ...func(*redshift.Options)) (*redshift.DescribePartnersOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribePartners"); err != nil {
		return nil, err
	}
	if err := f.check(in.AccountId, in.ClusterIdentifier, in.DatabaseName, in.PartnerName); err != nil {
		return nil, err
	}
	listed := map[string]map[string]any{}
	for _, key := range f.store.List(partnerType) {
		listed[key] = f.store.Resource(partnerType, key)
	}
	for key, props := range f.deleted {
		listed[key] = props
	}
	out := []types.PartnerIntegrationInfo{}
	for key, props := range listed {
		if awsapi.String(props, "ClusterIdentifier") != strings.ToLower(aws.ToString(in.ClusterIdentifier)) ||
			(in.DatabaseName != nil && awsapi.String(props, "DatabaseName") != *in.DatabaseName) ||
			(in.PartnerName != nil && awsapi.String(props, "PartnerName") != *in.PartnerName) {
			continue
		}
		delete(f.deleted, key)
		info := types.PartnerIntegrationInfo{
			DatabaseName: aws.String(awsapi.String(props, "DatabaseName")),
			PartnerName:  aws.String(awsapi.String(props, "PartnerName")),
			CreatedAt:    aws.Time(time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)),
			Status:       types.PartnerIntegrationStatus(awsapi.String(props, "Status")),
		}
		if message := awsapi.String(props, "StatusMessage"); message != "" {
			info.StatusMessage = aws.String(message)
		}
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b types.PartnerIntegrationInfo) int {
		return strings.Compare(aws.ToString(a.DatabaseName)+aws.ToString(a.PartnerName), aws.ToString(b.DatabaseName)+aws.ToString(b.PartnerName))
	})
	return &redshift.DescribePartnersOutput{PartnerIntegrationInfoList: out}, nil
}
