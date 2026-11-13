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

// Package dms has the native handler for AWS Database Migration Service (DMS) replication instances, which Cloud
// Control can't create in any region.
package dms

import (
	"context"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	dms "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"
	"github.com/aws/aws-sdk-go-v2/service/databasemigrationservice/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const instanceType = "AWS::DMS::ReplicationInstance"

// API is the part of the DMS client the handler uses.
type API interface {
	CreateReplicationInstance(ctx context.Context, params *dms.CreateReplicationInstanceInput, optFns ...func(*dms.Options)) (*dms.CreateReplicationInstanceOutput, error)
	DescribeReplicationInstances(ctx context.Context, params *dms.DescribeReplicationInstancesInput, optFns ...func(*dms.Options)) (*dms.DescribeReplicationInstancesOutput, error)
	ModifyReplicationInstance(ctx context.Context, params *dms.ModifyReplicationInstanceInput, optFns ...func(*dms.Options)) (*dms.ModifyReplicationInstanceOutput, error)
	DeleteReplicationInstance(ctx context.Context, params *dms.DeleteReplicationInstanceInput, optFns ...func(*dms.Options)) (*dms.DeleteReplicationInstanceOutput, error)
	AddTagsToResource(ctx context.Context, params *dms.AddTagsToResourceInput, optFns ...func(*dms.Options)) (*dms.AddTagsToResourceOutput, error)
	RemoveTagsFromResource(ctx context.Context, params *dms.RemoveTagsFromResourceInput, optFns ...func(*dms.Options)) (*dms.RemoveTagsFromResourceOutput, error)
	ListTagsForResource(ctx context.Context, params *dms.ListTagsForResourceInput, optFns ...func(*dms.Options)) (*dms.ListTagsForResourceOutput, error)
}

var inRegion = awsapi.InRegion[dms.Options]

var (
	_ native.Handler           = (*ReplicationInstance)(nil)
	_ native.IdentifierEncoder = (*ReplicationInstance)(nil)
)

// instanceStates are the states a replication instance's create and update wait for.
var instanceStates = awsapi.States{Done: []string{"available"}, Failed: []string{"failed", "incompatible-network"}}

// ReplicationInstance serves AWS::DMS::ReplicationInstance. Its identifier, Id, is ReplicationInstanceIdentifier,
// which DMS stores in lowercase, and which UCP generates when the template leaves it out.
type ReplicationInstance struct {
	api API
}

// NewReplicationInstance returns the handler for DMS replication instances.
func NewReplicationInstance(api API) *ReplicationInstance {
	return &ReplicationInstance{api: api}
}

// TypeName returns AWS::DMS::ReplicationInstance.
func (h *ReplicationInstance) TypeName() string {
	return instanceType
}

// Options returns how UCP uses the handler.
func (h *ReplicationInstance) Options() native.Options {
	return native.Options{
		List:         native.ListRegion,
		Recovery:     native.RecoverByIdentifier,
		NameProperty: "ReplicationInstanceIdentifier",
		Equivalent:   awsapi.CaseInsensitive(caseInsensitive...),
	}
}

// Identifier returns ReplicationInstanceIdentifier in lowercase, as DMS stores it.
func (h *ReplicationInstance) Identifier(properties map[string]any) (string, error) {
	return strings.ToLower(awsapi.String(properties, "ReplicationInstanceIdentifier")), nil
}

// caseInsensitive are the properties DMS returns in another case than the template may set them: names it stores in
// lowercase, and the maintenance window. ResourceIdentifier isn't one: the ARN keeps its case, such as
// rep:Example-App-ARN1, so another case is another value.
var caseInsensitive = []string{"ReplicationInstanceIdentifier", "ReplicationSubnetGroupIdentifier", "PreferredMaintenanceWindow"}

// describe returns the replication instance with the identifier, or ErrNotFound. An instance DMS reports as deleted
// is gone.
func (h *ReplicationInstance) describe(ctx context.Context, region string, identifier string) (*types.ReplicationInstance, error) {
	out, err := h.api.DescribeReplicationInstances(ctx, &dms.DescribeReplicationInstancesInput{
		Filters: []types.Filter{{Name: aws.String("replication-instance-id"), Values: []string{identifier}}},
	}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "ResourceNotFoundFault")
	}
	for i := range out.ReplicationInstances {
		instance := &out.ReplicationInstances[i]
		if strings.EqualFold(aws.ToString(instance.ReplicationInstanceIdentifier), identifier) && !deleted(instance) {
			return instance, nil
		}
	}
	return nil, native.ErrNotFound
}

func deleted(instance *types.ReplicationInstance) bool {
	return strings.EqualFold(aws.ToString(instance.ReplicationInstanceStatus), "deleted")
}

// Read returns the replication instance's properties.
func (h *ReplicationInstance) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	instance, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return h.properties(ctx, region, instance)
}

// properties returns the instance's properties. KmsKeyId and AllowMajorVersionUpgrade are write-only: DMS returns the
// key as an ARN, which a template's key ID or alias can't be matched with, and AllowMajorVersionUpgrade is only an
// input of a modify.
func (h *ReplicationInstance) properties(ctx context.Context, region string, i *types.ReplicationInstance) (map[string]any, error) {
	tags, err := h.api.ListTagsForResource(ctx, &dms.ListTagsForResourceInput{ResourceArn: i.ReplicationInstanceArn}, inRegion(region))
	if err != nil {
		return nil, err
	}
	groups := []string{}
	for _, g := range i.VpcSecurityGroups {
		groups = append(groups, aws.ToString(g.VpcSecurityGroupId))
	}
	p := map[string]any{
		"Id":                                    i.ReplicationInstanceIdentifier,
		"ReplicationInstanceIdentifier":         i.ReplicationInstanceIdentifier,
		"ReplicationInstanceClass":              i.ReplicationInstanceClass,
		"AllocatedStorage":                      i.AllocatedStorage,
		"AutoMinorVersionUpgrade":               i.AutoMinorVersionUpgrade,
		"AvailabilityZone":                      i.AvailabilityZone,
		"DnsNameServers":                        i.DnsNameServers,
		"EngineVersion":                         i.EngineVersion,
		"MultiAZ":                               i.MultiAZ,
		"NetworkType":                           i.NetworkType,
		"PreferredMaintenanceWindow":            i.PreferredMaintenanceWindow,
		"PubliclyAccessible":                    i.PubliclyAccessible,
		"ResourceIdentifier":                    resourceIdentifier(aws.ToString(i.ReplicationInstanceArn)),
		"VpcSecurityGroupIds":                   groups,
		"ReplicationInstancePrivateIpAddresses": strings.Join(i.ReplicationInstancePrivateIpAddresses, ","),
		"ReplicationInstancePublicIpAddresses":  strings.Join(i.ReplicationInstancePublicIpAddresses, ","),
		"Tags":                                  awsapi.TagListOf(tags.TagList),
	}
	if i.ReplicationSubnetGroup != nil {
		p["ReplicationSubnetGroupIdentifier"] = i.ReplicationSubnetGroup.ReplicationSubnetGroupIdentifier
	}
	return awsapi.EncodeObject(p)
}

// resourceIdentifier returns the ResourceIdentifier an instance's ARN ends with, such as Example-App-ARN1 for
// arn:aws:dms:eu-west-1:012345678901:rep:Example-App-ARN1. DMS generates one when the template leaves it out.
func resourceIdentifier(arn string) string {
	return arn[strings.LastIndex(arn, ":")+1:]
}

// Create creates the replication instance.
func (h *ReplicationInstance) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	input := &dms.CreateReplicationInstanceInput{}
	if err := awsapi.Decode(awsapi.Without(request.Properties, "Tags"), input); err != nil {
		return "", err
	}
	input.Tags = awsapi.SDKTags[types.Tag](awsapi.Tags(request.Properties))
	if _, err := h.api.CreateReplicationInstance(ctx, input, inRegion(request.Region)); err != nil {
		return "", err
	}
	return h.Identifier(request.Properties)
}

// modifiable are the properties ModifyReplicationInstance changes. Tags change through their own calls.
var modifiable = []string{
	"AllocatedStorage", "AutoMinorVersionUpgrade", "EngineVersion", "MultiAZ", "NetworkType",
	"PreferredMaintenanceWindow", "ReplicationInstanceClass", "VpcSecurityGroupIds",
}

// removedDefaults are the values a property goes back to when the template removes it. DMS keeps its value of any
// other removed property.
var removedDefaults = map[string]any{
	"AutoMinorVersionUpgrade": true,
	"MultiAZ":                 false,
}

// Update makes the patch's changes with ModifyReplicationInstance, applied at once, then adds and removes tags. A
// change of EngineVersion sends the template's AllowMajorVersionUpgrade. A change of AllowMajorVersionUpgrade alone
// changes nothing in AWS, because it only applies to the modify it is sent with.
func (h *ReplicationInstance) Update(ctx context.Context, request native.UpdateRequest) error {
	instance, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	changed := awsapi.Changed(request)
	values := map[string]any{}
	for _, name := range changed {
		if !slices.Contains(modifiable, name) {
			continue
		}
		value, ok := request.Desired[name]
		if !ok {
			if value, ok = removedDefaults[name]; !ok {
				continue
			}
		}
		values[name] = value
	}
	calls := []func() error{}
	if len(values) > 0 {
		input := &dms.ModifyReplicationInstanceInput{}
		if err := awsapi.Decode(values, input); err != nil {
			return err
		}
		input.ReplicationInstanceArn = instance.ReplicationInstanceArn
		input.ApplyImmediately = true
		if _, ok := values["EngineVersion"]; ok {
			input.AllowMajorVersionUpgrade = request.Desired["AllowMajorVersionUpgrade"] == true
		}
		calls = append(calls, func() error {
			_, err := h.api.ModifyReplicationInstance(ctx, input, opt)
			return err
		})
	}
	if slices.Contains(changed, "Tags") {
		calls = append(calls, h.tagCalls(ctx, instance.ReplicationInstanceArn, request, opt)...)
	}
	return awsapi.Calls(calls...)
}

// tagCalls returns the calls that add and remove tags to go from the request's current tags to the desired ones.
func (h *ReplicationInstance) tagCalls(ctx context.Context, arn *string, request native.UpdateRequest, opt func(*dms.Options)) []func() error {
	set, remove := awsapi.TagChanges(awsapi.Tags(request.Current), awsapi.Tags(request.Desired))
	calls := []func() error{}
	if len(set) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.AddTagsToResource(ctx, &dms.AddTagsToResourceInput{ResourceArn: arn, Tags: awsapi.SDKTags[types.Tag](set)}, opt)
			return err
		})
	}
	if len(remove) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.RemoveTagsFromResource(ctx, &dms.RemoveTagsFromResourceInput{ResourceArn: arn, TagKeys: remove}, opt)
			return err
		})
	}
	return calls
}

// Delete deletes the replication instance. An instance DMS is already deleting had a delete reach AWS before, so the
// delete succeeds.
func (h *ReplicationInstance) Delete(ctx context.Context, region string, identifier string) error {
	instance, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeleteReplicationInstance(ctx, &dms.DeleteReplicationInstanceInput{ReplicationInstanceArn: instance.ReplicationInstanceArn}, inRegion(region))
	if awsapi.HasCode(err, "InvalidResourceStateFault") {
		instance, describeErr := h.describe(ctx, region, identifier)
		if describeErr != nil {
			return describeErr
		}
		if strings.EqualFold(aws.ToString(instance.ReplicationInstanceStatus), "deleting") {
			return nil
		}
	}
	return awsapi.NotFound(err, "ResourceNotFoundFault")
}

// List returns every replication instance in the region.
func (h *ReplicationInstance) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var marker *string
	for {
		page, err := h.api.DescribeReplicationInstances(ctx, &dms.DescribeReplicationInstancesInput{Marker: marker}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.ReplicationInstances {
			instance := &page.ReplicationInstances[i]
			if deleted(instance) {
				continue
			}
			properties, err := h.properties(ctx, region, instance)
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: aws.ToString(instance.ReplicationInstanceIdentifier), Properties: properties})
		}
		if marker = page.Marker; marker == nil {
			return out, nil
		}
	}
}

// Progress reads the instance's status. A create or update waits for available with no modification pending, and a
// delete for the instance to be gone.
func (h *ReplicationInstance) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	instance, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := aws.ToString(instance.ReplicationInstanceStatus)
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: "deleted"}, nil
	}
	progress := instanceStates.Progress(status)
	if progress.Done && progress.Failure == nil && pending(instance.PendingModifiedValues) {
		return native.Progress{State: "modification pending", Waiting: "available"}, nil
	}
	return progress, nil
}

// pending reports whether DMS still has a modification to apply.
func pending(v *types.ReplicationPendingModifiedValues) bool {
	return v != nil && (v.AllocatedStorage != nil || v.EngineVersion != nil || v.MultiAZ != nil || v.NetworkType != nil || v.ReplicationInstanceClass != nil)
}
