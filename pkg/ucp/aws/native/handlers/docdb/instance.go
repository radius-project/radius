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

package docdb

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/aws/aws-sdk-go-v2/service/docdb/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const instanceType = "AWS::DocDB::DBInstance"

var (
	_ native.Handler           = (*Instance)(nil)
	_ native.IdentifierEncoder = (*Instance)(nil)
)

// instanceStates are the states an instance's create and update wait for.
var instanceStates = awsapi.States{Done: []string{"available"}, Failed: []string{"failed", "inaccessible-encryption-credentials", "incompatible-network", "incompatible-parameters", "storage-full"}}

// Instance serves AWS::DocDB::DBInstance. Its identifier, Id, is DBInstanceIdentifier, which UCP generates when the
// template leaves it out.
type Instance struct {
	api API
}

// NewInstance returns the handler for DocumentDB instances.
func NewInstance(api API) *Instance {
	return &Instance{api: api}
}

// TypeName returns AWS::DocDB::DBInstance.
func (h *Instance) TypeName() string {
	return instanceType
}

// Options returns how UCP uses the handler.
func (h *Instance) Options() native.Options {
	return native.Options{
		List:          native.ListRegion,
		Recovery:      native.RecoverByIdentifier,
		NameProperty:  "DBInstanceIdentifier",
		Equivalent:    equivalent,
		CreateTimeout: time.Hour,
		UpdateTimeout: time.Hour,
		DeleteTimeout: time.Hour,
	}
}

// Identifier returns DBInstanceIdentifier in lowercase, as DocumentDB stores it.
func (h *Instance) Identifier(properties map[string]any) (string, error) {
	return strings.ToLower(awsapi.String(properties, "DBInstanceIdentifier")), nil
}

// describe returns the instance with the identifier, ErrNotFound, or an OtherProductError when it doesn't run docdb.
func (h *Instance) describe(ctx context.Context, region string, identifier string) (*types.DBInstance, error) {
	out, err := h.api.DescribeDBInstances(ctx, &docdb.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "DBInstanceNotFound", "DBInstanceNotFoundFault")
	}
	if len(out.DBInstances) == 0 {
		return nil, native.ErrNotFound
	}
	instance := out.DBInstances[0]
	if engine := aws.ToString(instance.Engine); engine != Engine {
		return nil, &native.OtherProductError{Identifier: identifier, Product: engine}
	}
	return &instance, nil
}

// Read returns the instance's properties.
func (h *Instance) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	instance, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return h.properties(ctx, region, instance)
}

func (h *Instance) properties(ctx context.Context, region string, i *types.DBInstance) (map[string]any, error) {
	tags, err := h.api.ListTagsForResource(ctx, &docdb.ListTagsForResourceInput{ResourceName: i.DBInstanceArn}, inRegion(region))
	if err != nil {
		return nil, err
	}
	p := map[string]any{
		"Id":                         i.DBInstanceIdentifier,
		"DBInstanceIdentifier":       i.DBInstanceIdentifier,
		"DBClusterIdentifier":        i.DBClusterIdentifier,
		"DBInstanceClass":            i.DBInstanceClass,
		"AvailabilityZone":           i.AvailabilityZone,
		"AutoMinorVersionUpgrade":    i.AutoMinorVersionUpgrade,
		"CACertificateIdentifier":    i.CACertificateIdentifier,
		"EnablePerformanceInsights":  i.PerformanceInsightsEnabled,
		"PreferredMaintenanceWindow": i.PreferredMaintenanceWindow,
		"Tags":                       tagList(tags.TagList),
	}
	if i.Endpoint != nil {
		p["Endpoint"] = i.Endpoint.Address
		if i.Endpoint.Port != nil {
			// The schema declares Port a string, as CloudFormation returns it.
			p["Port"] = strconv.Itoa(int(*i.Endpoint.Port))
		}
	}
	return awsapi.EncodeObject(p)
}

// Create creates the instance in its cluster.
func (h *Instance) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	input := &docdb.CreateDBInstanceInput{}
	if err := awsapi.Decode(awsapi.Without(request.Properties, "Tags"), input); err != nil {
		return "", err
	}
	input.Engine = aws.String(Engine)
	input.Tags = sdkTags(awsapi.Tags(request.Properties))
	if _, err := h.api.CreateDBInstance(ctx, input, inRegion(request.Region)); err != nil {
		return "", err
	}
	return h.Identifier(request.Properties)
}

// modifiableInstance are the properties ModifyDBInstance changes.
var modifiableInstance = []string{
	"AutoMinorVersionUpgrade", "CACertificateIdentifier", "CertificateRotationRestart", "DBInstanceClass",
	"EnablePerformanceInsights", "PreferredMaintenanceWindow",
}

// removedInstanceDefaults are the values a removed property goes back to, AWS's defaults. AWS keeps the value of any
// other removed property: CACertificateIdentifier and PreferredMaintenanceWindow have no default to go back to.
var removedInstanceDefaults = map[string]any{
	"AutoMinorVersionUpgrade":   true,
	"EnablePerformanceInsights": false,
}

// Update makes the patch's changes with ModifyDBInstance, applied at once, then adds and removes tags.
func (h *Instance) Update(ctx context.Context, request native.UpdateRequest) error {
	instance, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	changed := awsapi.Changed(request)
	values := map[string]any{}
	for _, name := range changed {
		if !slices.Contains(modifiableInstance, name) {
			continue
		}
		value, ok := request.Desired[name]
		if !ok {
			if value, ok = removedInstanceDefaults[name]; !ok {
				continue
			}
		}
		values[name] = value
	}
	if _, ok := values["CACertificateIdentifier"]; ok {
		// CertificateRotationRestart is write-only, so the patch leaves it out when it didn't change, but it says
		// whether this CA change restarts the instance.
		if restart, ok := request.Desired["CertificateRotationRestart"]; ok {
			values["CertificateRotationRestart"] = restart
		}
	}
	calls := []func() error{}
	if len(values) > 0 {
		input := &docdb.ModifyDBInstanceInput{}
		if err := awsapi.Decode(values, input); err != nil {
			return err
		}
		input.DBInstanceIdentifier = aws.String(request.Identifier)
		input.ApplyImmediately = aws.Bool(true)
		calls = append(calls, func() error {
			_, err := h.api.ModifyDBInstance(ctx, input, opt)
			return err
		})
	}
	if slices.Contains(changed, "Tags") {
		calls = append(calls, tagCalls(ctx, h.api, instance.DBInstanceArn, request, opt)...)
	}
	return awsapi.Calls(calls...)
}

// Delete deletes the instance. An instance AWS is already deleting had a delete reach AWS before, so the delete
// succeeds.
func (h *Instance) Delete(ctx context.Context, region string, identifier string) error {
	if _, err := h.describe(ctx, region, identifier); err != nil {
		return err
	}
	_, err := h.api.DeleteDBInstance(ctx, &docdb.DeleteDBInstanceInput{DBInstanceIdentifier: aws.String(identifier)}, inRegion(region))
	if awsapi.HasCode(err, "InvalidDBInstanceState", "InvalidDBInstanceStateFault") {
		instance, describeErr := h.describe(ctx, region, identifier)
		if describeErr != nil {
			return describeErr
		}
		if strings.EqualFold(aws.ToString(instance.DBInstanceStatus), "deleting") {
			return nil
		}
	}
	return awsapi.NotFound(err, "DBInstanceNotFound", "DBInstanceNotFoundFault")
}

// List returns every DocumentDB instance in the region. Instances of other products are left out.
func (h *Instance) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var marker *string
	for {
		page, err := h.api.DescribeDBInstances(ctx, &docdb.DescribeDBInstancesInput{
			Filters: []types.Filter{{Name: aws.String("engine"), Values: []string{Engine}}},
			Marker:  marker,
		}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.DBInstances {
			instance := &page.DBInstances[i]
			if aws.ToString(instance.Engine) != Engine {
				continue
			}
			properties, err := h.properties(ctx, region, instance)
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: aws.ToString(instance.DBInstanceIdentifier), Properties: properties})
		}
		if marker = page.Marker; marker == nil {
			return out, nil
		}
	}
}

// Progress reads the instance's status. A create or update waits for available, and a delete for the instance to be
// gone.
func (h *Instance) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	instance, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := aws.ToString(instance.DBInstanceStatus)
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: "deleted"}, nil
	}
	return instanceStates.Progress(status), nil
}
