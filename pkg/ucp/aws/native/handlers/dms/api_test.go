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

package dms

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	dms "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"
	"github.com/aws/aws-sdk-go-v2/service/databasemigrationservice/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/dms/latest/APIReference/"
	commonErrors = apiReference + "CommonErrors.html"
	tagType      = apiReference + "API_Tag.html"
	userGuide    = "https://docs.aws.amazon.com/dms/latest/userguide/"
	quotas       = userGuide + "CHAP_Limits.html"
	creating     = userGuide + "CHAP_ReplicationInstance.Creating.html"
	modifying    = userGuide + "CHAP_ReplicationInstance.Modifying.html"

	// validationUnclear is what the pages leave unclear about the error for a value outside its limits.
	validationUnclear = "The pages name no error for a value outside its limits. The fake returns ValidationError, which " +
		"Common Errors gives for input that doesn't meet its constraints."
)

// createInput returns a create of the instance id that follows every rule, changed by change when it is set.
func createInput(id string, change func(in *dms.CreateReplicationInstanceInput)) *dms.CreateReplicationInstanceInput {
	in := &dms.CreateReplicationInstanceInput{
		ReplicationInstanceIdentifier: aws.String(id),
		ReplicationInstanceClass:      aws.String("dms.t3.micro"),
		AllocatedStorage:              aws.Int32(50),
		AutoMinorVersionUpgrade:       aws.Bool(false),
		EngineVersion:                 aws.String("3.5.3"),
	}
	if change != nil {
		change(in)
	}
	return in
}

// create creates the instance id, changed by change, with f.
func create(ctx context.Context, f *fakeAPI, id string, change func(in *dms.CreateReplicationInstanceInput)) error {
	_, err := f.CreateReplicationInstance(ctx, createInput(id, change))
	return err
}

// createWith returns a call that creates app-dms, changed by change, with a fake DMS API backed by store.
func createWith(change func(in *dms.CreateReplicationInstanceInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		return create(ctx, newFakeAPI(store), "app-dms", change)
	}
}

// describe returns the instance id as f describes it.
func describe(ctx context.Context, f *fakeAPI, id string) (types.ReplicationInstance, error) {
	out, err := f.DescribeReplicationInstances(ctx, &dms.DescribeReplicationInstancesInput{
		Filters: []types.Filter{{Name: aws.String("replication-instance-id"), Values: []string{id}}},
	})
	if err != nil {
		return types.ReplicationInstance{}, err
	}
	if len(out.ReplicationInstances) != 1 {
		return types.ReplicationInstance{}, fmt.Errorf("found %d instances", len(out.ReplicationInstances))
	}
	return out.ReplicationInstances[0], nil
}

// created creates app-dms, changed by change, with f and returns its ARN.
func created(ctx context.Context, f *fakeAPI, change func(in *dms.CreateReplicationInstanceInput)) (*string, error) {
	if err := create(ctx, f, "app-dms", change); err != nil {
		return nil, err
	}
	instance, err := describe(ctx, f, "app-dms")
	return instance.ReplicationInstanceArn, err
}

// createdWith returns the instance a create of app-dms, changed by change, makes.
func createdWith(ctx context.Context, store *awsfake.CloudControl, change func(in *dms.CreateReplicationInstanceInput)) (types.ReplicationInstance, error) {
	f := newFakeAPI(store)
	if err := create(ctx, f, "app-dms", change); err != nil {
		return types.ReplicationInstance{}, err
	}
	return describe(ctx, f, "app-dms")
}

// modify creates app-dms with f and modifies it, applied immediately and changed by change.
func modify(ctx context.Context, f *fakeAPI, change func(in *dms.ModifyReplicationInstanceInput)) error {
	arn, err := created(ctx, f, nil)
	if err != nil {
		return err
	}
	in := &dms.ModifyReplicationInstanceInput{ReplicationInstanceArn: arn, ApplyImmediately: true}
	change(in)
	_, err = f.ModifyReplicationInstance(ctx, in)
	return err
}

// modifyWith returns a call that creates app-dms with a fake DMS API backed by store, and modifies it as modify does.
func modifyWith(change func(in *dms.ModifyReplicationInstanceInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		return modify(ctx, newFakeAPI(store), change)
	}
}

// describeWith returns a call that describes instances with MaxRecords, with a fake DMS API backed by store.
func describeWith(maxRecords int32) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		_, err := newFakeAPI(store).DescribeReplicationInstances(ctx, &dms.DescribeReplicationInstancesInput{MaxRecords: aws.Int32(maxRecords)})
		return err
	}
}

// tagWith returns a call that creates app-dms and adds the tags to it, with a fake DMS API backed by store.
func tagWith(tags ...types.Tag) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeAPI(store)
		arn, err := created(ctx, f, nil)
		if err != nil {
			return err
		}
		_, err = f.AddTagsToResource(ctx, &dms.AddTagsToResourceInput{ResourceArn: arn, Tags: tags})
		return err
	}
}

func tag(key string, value string) types.Tag {
	return types.Tag{Key: aws.String(key), Value: aws.String(value)}
}

// missingARN is the ARN of an instance that doesn't exist.
var missingARN = aws.String(arnPrefix + "MISSING")

// tagRulesOf returns the rules of the Tag data type for a call that sets tags, made by call with the tag.
func tagRulesOf(call func(t types.Tag) func(ctx context.Context, store *awsfake.CloudControl) error) []nativetest.Rule {
	return []nativetest.Rule{
		{Rule: "a tag key is 1 to 128 characters", Doc: tagType, Code: "ValidationError", Break: call(tag(strings.Repeat("k", 129), "a"))},
		{Rule: "a tag value is at most 256 characters", Doc: tagType, Code: "ValidationError", Break: call(tag("team", strings.Repeat("v", 257)))},
		{Rule: "a tag key or value doesn't start with aws: or dms:", Doc: tagType, Code: "ValidationError", Break: call(tag("dms:team", "a"))},
		{
			Rule: "a tag key or value has only letters, digits, white space, and _ . : / = + -",
			Doc:  tagType, Code: "ValidationError", Break: call(tag("team", "a@b")),
		},
	}
}

// createRules are the rules of CreateReplicationInstance.
var createRules = []nativetest.Rule{
	{
		Rule: "ReplicationInstanceIdentifier is 1 to 63 letters, digits, or hyphens",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.ReplicationInstanceIdentifier = aws.String(strings.Repeat("a", 64))
		}),
	},
	{
		Rule: "ReplicationInstanceIdentifier starts with a letter",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) { in.ReplicationInstanceIdentifier = aws.String("1app") }),
	},
	{
		Rule: "ReplicationInstanceIdentifier doesn't end with a hyphen",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) { in.ReplicationInstanceIdentifier = aws.String("app-") }),
	},
	{
		Rule: "ReplicationInstanceIdentifier has no two hyphens in a row",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.ReplicationInstanceIdentifier = aws.String("app--dms")
		}),
	},
	{
		Rule: "ReplicationInstanceIdentifier is stored in lowercase",
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			if err := create(ctx, f, "App-DMS", nil); err != nil {
				return err
			}
			instance, err := describe(ctx, f, "app-dms")
			if err != nil {
				return err
			}
			if got := aws.ToString(instance.ReplicationInstanceIdentifier); got != "app-dms" {
				return fmt.Errorf("the identifier is %s", got)
			}
			return nil
		},
	},
	{
		Rule: "ReplicationInstanceIdentifier is stored in lowercase, so APP-DMS already exists when app-dms does",
		Code: "ResourceAlreadyExistsFault",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			if err := create(ctx, f, "app-dms", nil); err != nil {
				return err
			}
			return create(ctx, f, "APP-DMS", nil)
		},
	},
	{
		Rule: "ResourceIdentifier is up to 31 ASCII letters, digits, or hyphens, starting with a letter",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.ResourceIdentifier = aws.String(strings.Repeat("A", 32))
		}),
	},
	{
		Rule: "ResourceIdentifier doesn't end with a hyphen or have two hyphens in a row",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) { in.ResourceIdentifier = aws.String("App--Rep") }),
	},
	{
		Rule: "ResourceIdentifier starts with a letter, so an explicit empty one isn't a value left out",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) { in.ResourceIdentifier = aws.String("") }),
	},
	{
		Rule: "DMS generates a ResourceIdentifier for the end of the ARN when it is left out",
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			instance, err := createdWith(ctx, store, nil)
			if err != nil {
				return err
			}
			if resourceIdentifier(aws.ToString(instance.ReplicationInstanceArn)) == "" {
				return fmt.Errorf("the ARN %s has no resource identifier", aws.ToString(instance.ReplicationInstanceArn))
			}
			return nil
		},
	},
	{
		Rule: "ReplicationInstanceClass is at most 30 characters",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.ReplicationInstanceClass = aws.String("dms." + strings.Repeat("x", 27))
		}),
	},
	{
		Rule: "ReplicationInstanceClass's limit counts characters, not bytes, so 30 non-ASCII characters are within it",
		Check: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.ReplicationInstanceClass = aws.String(strings.Repeat("é", 30))
		}),
	},
	{
		Rule: "DnsNameServers lists up to four name servers",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.DnsNameServers = aws.String("1.1.1.1,2.2.2.2,3.3.3.3,4.4.4.4,5.5.5.5")
		}),
	},
	{
		Rule: "AvailabilityZone can't be set when MultiAZ is true",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.MultiAZ = aws.Bool(true)
			in.AvailabilityZone = aws.String("us-west-2a")
		}),
	},
	{
		Rule: "AvailabilityZone can't be set when MultiAZ is true, even to an empty value",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.MultiAZ = aws.Bool(true)
			in.AvailabilityZone = aws.String("")
		}),
	},
	{
		Rule: "PreferredMaintenanceWindow has the format ddd:hh24:mi-ddd:hh24:mi, with days Mon to Sun",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.PreferredMaintenanceWindow = aws.String("sun:06:00-day:06:30")
		}),
	},
	{
		Rule: "PreferredMaintenanceWindow is at least 30 minutes",
		Doc:  commonErrors, Code: "ValidationError",
		Break: createWith(func(in *dms.CreateReplicationInstanceInput) {
			in.PreferredMaintenanceWindow = aws.String("sun:06:00-sun:06:20")
		}),
	},
	{
		Rule: "the replication instances of a region have at most 30,000 GB of storage, by default",
		Doc:  quotas, Code: "StorageQuotaExceededFault",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			if err := create(ctx, f, "first", func(in *dms.CreateReplicationInstanceInput) { in.AllocatedStorage = aws.Int32(20000) }); err != nil {
				return err
			}
			return create(ctx, f, "second", func(in *dms.CreateReplicationInstanceInput) { in.AllocatedStorage = aws.Int32(10001) })
		},
	},
	{
		Rule: "a region has at most 60 replication instances, by default",
		Doc:  quotas, Code: "ResourceQuotaExceededFault",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			for n := range instanceQuota {
				if err := create(ctx, f, "app-"+strconv.Itoa(n), func(in *dms.CreateReplicationInstanceInput) { in.AllocatedStorage = aws.Int32(5) }); err != nil {
					return err
				}
			}
			return create(ctx, f, "one-more", func(in *dms.CreateReplicationInstanceInput) { in.AllocatedStorage = aws.Int32(5) })
		},
	},
	{
		Rule: "AutoMinorVersionUpgrade and PubliclyAccessible default to true",
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			instance, err := createdWith(ctx, store, func(in *dms.CreateReplicationInstanceInput) { in.AutoMinorVersionUpgrade = nil })
			if err != nil {
				return err
			}
			if !instance.AutoMinorVersionUpgrade || !instance.PubliclyAccessible {
				return fmt.Errorf("AutoMinorVersionUpgrade is %t and PubliclyAccessible is %t", instance.AutoMinorVersionUpgrade, instance.PubliclyAccessible)
			}
			return nil
		},
	},
	{
		Rule: "EngineVersion defaults to the latest engine version",
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			instance, err := createdWith(ctx, store, func(in *dms.CreateReplicationInstanceInput) { in.EngineVersion = nil })
			if err != nil {
				return err
			}
			if got := aws.ToString(instance.EngineVersion); got != defaultEngineVersion {
				return fmt.Errorf("the engine version is %q", got)
			}
			return nil
		},
	},
	{
		Rule: "with AutoMinorVersionUpgrade, DMS uses the default engine version in place of an older EngineVersion, and without it, the EngineVersion",
		Doc:  creating,
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			for id, autoUpgrade := range map[string]bool{"upgraded": true, "kept": false} {
				if err := create(ctx, f, id, func(in *dms.CreateReplicationInstanceInput) { in.AutoMinorVersionUpgrade = aws.Bool(autoUpgrade) }); err != nil {
					return err
				}
			}
			upgraded, err := describe(ctx, f, "upgraded")
			if err != nil {
				return err
			}
			kept, err := describe(ctx, f, "kept")
			if err != nil {
				return err
			}
			if aws.ToString(upgraded.EngineVersion) != defaultEngineVersion || aws.ToString(kept.EngineVersion) != "3.5.3" {
				return fmt.Errorf("the engine versions are %s and %s", aws.ToString(upgraded.EngineVersion), aws.ToString(kept.EngineVersion))
			}
			return nil
		},
	},
	{
		Rule: "AvailabilityZone defaults to one DMS picks, and PreferredMaintenanceWindow to a 30-minute window DMS picks",
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			instance, err := createdWith(ctx, store, nil)
			if err != nil {
				return err
			}
			if aws.ToString(instance.AvailabilityZone) == "" || !validWindow(aws.ToString(instance.PreferredMaintenanceWindow)) {
				return fmt.Errorf("the zone is %q and the window %q", aws.ToString(instance.AvailabilityZone), aws.ToString(instance.PreferredMaintenanceWindow))
			}
			return nil
		},
	},
}

// modifyRules are the rules of ModifyReplicationInstance.
var modifyRules = []nativetest.Rule{
	{
		Rule: "the instance exists",
		Code: "ResourceNotFoundFault",
		Break: modifyWith(func(in *dms.ModifyReplicationInstanceInput) {
			in.ReplicationInstanceArn = missingARN
			in.ReplicationInstanceClass = aws.String("dms.t3.small")
		}),
	},
	{
		Rule: "AllowMajorVersionUpgrade must be true for an EngineVersion of another major version",
		Doc:  commonErrors, Code: "ValidationError",
		Break: modifyWith(func(in *dms.ModifyReplicationInstanceInput) { in.EngineVersion = aws.String("4.0.0") }),
	},
	{
		Rule: "DMS doesn't count a change such as 3.4 to 3.5 as a major version change, so it needs no AllowMajorVersionUpgrade",
		Doc:  modifying,
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			return modify(ctx, newFakeAPI(store), func(in *dms.ModifyReplicationInstanceInput) { in.EngineVersion = aws.String("3.6.1") })
		},
	},
	{
		Rule: "ReplicationInstanceClass is at most 30 characters",
		Doc:  commonErrors, Code: "ValidationError",
		Break: modifyWith(func(in *dms.ModifyReplicationInstanceInput) {
			in.ReplicationInstanceClass = aws.String("dms." + strings.Repeat("x", 27))
		}),
	},
	{
		Rule: "ReplicationInstanceClass's limit counts characters, not bytes, so 30 non-ASCII characters are within it",
		Check: modifyWith(func(in *dms.ModifyReplicationInstanceInput) {
			in.ReplicationInstanceClass = aws.String(strings.Repeat("é", 30))
		}),
	},
	{
		Rule: "PreferredMaintenanceWindow has the format ddd:hh24:mi-ddd:hh24:mi and is at least 30 minutes",
		Doc:  commonErrors, Code: "ValidationError",
		Break: modifyWith(func(in *dms.ModifyReplicationInstanceInput) {
			in.PreferredMaintenanceWindow = aws.String("sun:06:00-sun:06:29")
		}),
	},
	{
		Rule: "AllocatedStorage can't decrease",
		Doc:  modifying, Code: "ValidationError",
		Break: modifyWith(func(in *dms.ModifyReplicationInstanceInput) { in.AllocatedStorage = aws.Int32(49) }),
	},
	{
		Rule: "AllocatedStorage can't decrease, and an explicit 0 is a decrease, not a value left out",
		Doc:  modifying, Code: "ValidationError",
		Break: modifyWith(func(in *dms.ModifyReplicationInstanceInput) { in.AllocatedStorage = aws.Int32(0) }),
	},
	{
		Rule: "the replication instances of a region have at most 30,000 GB of storage, by default",
		Doc:  quotas, Code: "StorageQuotaExceededFault",
		Break: modifyWith(func(in *dms.ModifyReplicationInstanceInput) { in.AllocatedStorage = aws.Int32(30001) }),
	},
	{
		Rule: "without ApplyImmediately, a change of the class, storage, engine version, Multi-AZ, or network type waits for the maintenance window in PendingModifiedValues",
		Doc:  modifying,
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			err := modify(ctx, f, func(in *dms.ModifyReplicationInstanceInput) {
				in.ApplyImmediately = false
				in.ReplicationInstanceClass = aws.String("dms.t3.small")
			})
			if err != nil {
				return err
			}
			instance, err := describe(ctx, f, "app-dms")
			if err != nil {
				return err
			}
			if aws.ToString(instance.ReplicationInstanceClass) != "dms.t3.micro" || instance.PendingModifiedValues == nil ||
				aws.ToString(instance.PendingModifiedValues.ReplicationInstanceClass) != "dms.t3.small" {
				return fmt.Errorf("the class is %s, with pending values %+v", aws.ToString(instance.ReplicationInstanceClass), instance.PendingModifiedValues)
			}
			return nil
		},
	},
	{
		Rule: "a modify applied immediately also applies the pending changes",
		Doc:  modifying,
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			err := modify(ctx, f, func(in *dms.ModifyReplicationInstanceInput) {
				in.ApplyImmediately = false
				in.ReplicationInstanceClass = aws.String("dms.t3.small")
			})
			if err != nil {
				return err
			}
			instance, err := describe(ctx, f, "app-dms")
			if err != nil {
				return err
			}
			_, err = f.ModifyReplicationInstance(ctx, &dms.ModifyReplicationInstanceInput{
				ReplicationInstanceArn: instance.ReplicationInstanceArn, ApplyImmediately: true, AllocatedStorage: aws.Int32(60),
			})
			if err != nil {
				return err
			}
			if instance, err = describe(ctx, f, "app-dms"); err != nil {
				return err
			}
			if aws.ToString(instance.ReplicationInstanceClass) != "dms.t3.small" || instance.AllocatedStorage != 60 || instance.PendingModifiedValues != nil {
				return fmt.Errorf("the class is %s and the storage %d, with pending values %+v", aws.ToString(instance.ReplicationInstanceClass), instance.AllocatedStorage, instance.PendingModifiedValues)
			}
			return nil
		},
	},
	{
		Rule: "a change of AutoMinorVersionUpgrade or PreferredMaintenanceWindow applies as soon as possible, without ApplyImmediately",
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			err := modify(ctx, f, func(in *dms.ModifyReplicationInstanceInput) {
				in.ApplyImmediately = false
				in.AutoMinorVersionUpgrade = aws.Bool(true)
				in.PreferredMaintenanceWindow = aws.String("tue:04:00-tue:04:30")
			})
			if err != nil {
				return err
			}
			instance, err := describe(ctx, f, "app-dms")
			if err != nil {
				return err
			}
			if !instance.AutoMinorVersionUpgrade || aws.ToString(instance.PreferredMaintenanceWindow) != "tue:04:00-tue:04:30" {
				return fmt.Errorf("AutoMinorVersionUpgrade is %t and the window %s", instance.AutoMinorVersionUpgrade, aws.ToString(instance.PreferredMaintenanceWindow))
			}
			return nil
		},
	},
}

// describeRules are the rules of DescribeReplicationInstances.
var describeRules = []nativetest.Rule{
	{Rule: "MaxRecords is at least 20", Doc: commonErrors, Code: "ValidationError", Break: describeWith(19)},
	{Rule: "MaxRecords is at most 100", Doc: commonErrors, Code: "ValidationError", Break: describeWith(101)},
	{Rule: "MaxRecords is at least 20, and an explicit 0 isn't a value left out", Doc: commonErrors, Code: "ValidationError", Break: describeWith(0)},
	{
		Rule: "a filter's name is replication-instance-arn, replication-instance-id, replication-instance-class, or engine-version",
		Doc:  commonErrors, Code: "ValidationError",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			_, err := newFakeAPI(store).DescribeReplicationInstances(ctx, &dms.DescribeReplicationInstancesInput{
				Filters: []types.Filter{{Name: aws.String("rep-instance-id"), Values: []string{"app-dms"}}},
			})
			return err
		},
	},
	{
		Rule: "MaxRecords defaults to 100, and a page with more records after it has a Marker",
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			for n := range describePageSize + 1 {
				store.Put(instanceType, "app-"+strconv.Itoa(n), map[string]any{"ReplicationInstanceClass": "dms.t3.micro"})
			}
			out, err := newFakeAPI(store).DescribeReplicationInstances(ctx, &dms.DescribeReplicationInstancesInput{})
			if err != nil {
				return err
			}
			if len(out.ReplicationInstances) != describePageSize || out.Marker == nil {
				return fmt.Errorf("the page has %d instances and the marker %v", len(out.ReplicationInstances), out.Marker)
			}
			return nil
		},
	},
}

// replicationInstanceAPI records the DMS calls ReplicationInstance makes.
var replicationInstanceAPI = map[string]nativetest.Call{
	"CreateReplicationInstance": {
		Doc: apiReference + "API_CreateReplicationInstance.html",
		Rules: append(createRules, tagRulesOf(func(t types.Tag) func(ctx context.Context, store *awsfake.CloudControl) error {
			return createWith(func(in *dms.CreateReplicationInstanceInput) { in.Tags = []types.Tag{t} })
		})...),
		Unclear: validationUnclear + " The page gives no AllocatedStorage limits or default, so the fake checks only the " +
			"region's storage quota, and reports no storage for a create without it. It says both that EngineVersion " +
			"defaults to the latest version and to the default version in the release notes; the fake uses one version. " +
			"It doesn't list NetworkType's values, or MultiAZ's default; the fake accepts any network type and reports " +
			"no MultiAZ, and the handler sends false when the template removes it. KmsKeyId defaults to the account's " +
			"default key; the fake doesn't report a key, which the handler treats as write-only. There is no client " +
			"token: the identifier, stored in lowercase, makes a create unique. The page's sample request sends empty " +
			"AvailabilityZone and PreferredMaintenanceWindow values, as if AWS took them as left out; the fake follows " +
			"the stated limits instead, and counts an explicit empty value as set.",
	},
	"ModifyReplicationInstance": {
		Doc:   apiReference + "API_ModifyReplicationInstance.html",
		Rules: modifyRules,
		Unclear: validationUnclear + " That includes a smaller AllocatedStorage and a major version change without " +
			"AllowMajorVersionUpgrade, where the fake takes the first number of a version as its major version. The " +
			"page doesn't say which states allow a modify; the fake accepts one in any state. It gives ApplyImmediately " +
			"no default; the user guide says changes not applied immediately are " +
			"queued, so the fake queues them when it is false or left out. It doesn't say whether a change of " +
			"VpcSecurityGroupIds waits for the maintenance window; the fake applies it at once. It doesn't say whether " +
			"turning on MultiAZ for an instance created with an AvailabilityZone is allowed; the fake allows it. The " +
			"page's sample request sends AllocatedStorage 0 and empty values, as if AWS took them as left out; the fake " +
			"follows the stated limits instead, so an explicit 0 is a decrease.",
	},
	"DescribeReplicationInstances": {
		Doc:   apiReference + "API_DescribeReplicationInstances.html",
		Rules: describeRules,
		Unclear: validationUnclear + " The page doesn't say what a filter that matches no instance returns; the fake " +
			"returns ResourceNotFoundFault, as a recorded run of the handler shows AWS does. It doesn't say whether " +
			"replication-instance-id ignores case; the fake matches only the identifier as stored, in lowercase, which " +
			"is how the handler sends it. The fake filters only by replication-instance-id, the one filter the handler " +
			"sends. The page's sample request sends MaxRecords 0; the fake follows the stated limit of 20 to 100 " +
			"instead.",
	},
	"DeleteReplicationInstance": {
		Doc: apiReference + "API_DeleteReplicationInstance.html",
		Rules: []nativetest.Rule{{
			Rule: "the instance exists",
			Code: "ResourceNotFoundFault",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				_, err := newFakeAPI(store).DeleteReplicationInstance(ctx, &dms.DeleteReplicationInstanceInput{ReplicationInstanceArn: missingARN})
				return err
			},
		}},
		Unclear: "The page says an instance's migration tasks must be deleted first, but names no error for it; the fake " +
			"has no tasks. It doesn't say what a delete of an instance DMS is already deleting returns; the fake returns " +
			"InvalidResourceStateFault, which the handler treats as a delete that already reached AWS.",
	},
	"AddTagsToResource": {
		Doc: apiReference + "API_AddTagsToResource.html",
		Rules: append(tagRulesOf(func(t types.Tag) func(ctx context.Context, store *awsfake.CloudControl) error {
			return tagWith(t)
		}), nativetest.Rule{
			Rule: "the resource exists",
			Code: "ResourceNotFoundFault",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				_, err := newFakeAPI(store).AddTagsToResource(ctx, &dms.AddTagsToResourceInput{ResourceArn: missingARN, Tags: []types.Tag{tag("team", "a")}})
				return err
			},
		}),
		Unclear: validationUnclear + " The Tag page says a value is 1 to 256 characters, but also that it is optional, " +
			"and its pattern allows an empty one; the fake accepts an empty value. It doesn't say whether aws: and dms: " +
			"are matched without case; the fake rejects only those prefixes in lowercase.",
	},
	"RemoveTagsFromResource": {
		Doc: apiReference + "API_RemoveTagsFromResource.html",
		Rules: []nativetest.Rule{{
			Rule: "the resource exists",
			Code: "ResourceNotFoundFault",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				_, err := newFakeAPI(store).RemoveTagsFromResource(ctx, &dms.RemoveTagsFromResourceInput{ResourceArn: missingARN, TagKeys: []string{"team"}})
				return err
			},
		}},
		Unclear: "The page doesn't say what removing a key the resource doesn't have does; the fake ignores the key.",
	},
	"ListTagsForResource": {
		Doc: apiReference + "API_ListTagsForResource.html",
		Rules: []nativetest.Rule{{
			Rule: "the resource exists",
			Code: "ResourceNotFoundFault",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				_, err := newFakeAPI(store).ListTagsForResource(ctx, &dms.ListTagsForResourceInput{ResourceArn: missingARN})
				return err
			},
		}},
	},
}
