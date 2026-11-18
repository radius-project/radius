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

package s3tables

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"
	"github.com/aws/aws-sdk-go-v2/service/s3tables/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

const (
	// missingPrefix starts the names of tables AWS doesn't have.
	missingPrefix = "missing"

	// versionType is the pseudo type the store keeps each table's configuration version under.
	versionType = "version::" + tableReplicationType

	badRequestCode = "BadRequestException"
	conflictCode   = "ConflictException"
)

var (
	// tableArnPattern, roleArnPattern, and bucketArnPattern are the patterns S3 Tables' API reference gives for a
	// table ARN, the replication role, and a destination table bucket ARN.
	tableArnPattern  = regexp.MustCompile(`^arn:aws[-a-z0-9]*:[a-z0-9]+:[-a-z0-9]*:[0-9]{12}:bucket/[a-z0-9_-]{3,63}/table/[a-zA-Z0-9-_]{1,255}$`)
	roleArnPattern   = regexp.MustCompile(`^arn:.+:iam::[0-9]{12}:role/.+$`)
	bucketArnPattern = regexp.MustCompile(`^arn:aws[-a-z0-9]*:[a-z0-9]+:[-a-z0-9]*:[0-9]{12}:bucket/[a-z0-9_-]{3,63}$`)
)

// fakeAPI is an in-memory S3 Tables API for table replication configurations. It keeps them in an awsfake store, in
// the type's property format, so the suite's Cloud Control sees the same resources. Each put gives the configuration
// a new version token, which the store keeps under versionType. Every table exists, except those whose name starts with missingPrefix.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu    sync.Mutex
	calls []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, errs: map[string]error{}}
}

func fault(code string) error {
	return nativefake.ClientError(code, code)
}

func (f *fakeAPI) begin(method string) error {
	f.calls = append(f.calls, method)
	return f.errs[method]
}

// table returns the error S3 Tables returns for a table ARN it doesn't take or a table it doesn't have.
func table(arn string) error {
	if len(arn) > 2048 || !tableArnPattern.MatchString(arn) {
		return fault(badRequestCode)
	}
	if name := arn[strings.LastIndex(arn, "/")+1:]; strings.HasPrefix(name, missingPrefix) {
		return fault("NotFoundException")
	}
	return nil
}

// version returns the current version token of the table's configuration.
func (f *fakeAPI) version(arn string) string {
	n, _ := f.store.Resource(versionType, arn)["Version"].(float64)
	return arn + "#" + strconv.Itoa(int(n))
}

// nextVersion gives the table's configuration a new version token.
func (f *fakeAPI) nextVersion(arn string) {
	n, _ := f.store.Resource(versionType, arn)["Version"].(float64)
	f.store.Put(versionType, arn, map[string]any{"Version": n + 1})
}

// checkVersion returns ConflictException when token isn't the configuration's current version token.
func (f *fakeAPI) checkVersion(arn string, token *string) error {
	if aws.ToString(token) != f.version(arn) {
		return fault(conflictCode)
	}
	return nil
}

// validConfiguration returns BadRequestException for a configuration outside the limits S3 Tables' API reference
// gives: a role of 20 to 2048 characters that matches its pattern, exactly one rule, and one to five destinations
// that match the table bucket ARN pattern.
func validConfiguration(c *types.TableReplicationConfiguration) error {
	if c == nil {
		return fault(badRequestCode)
	}
	role := aws.ToString(c.Role)
	if len(role) < 20 || len(role) > 2048 || !roleArnPattern.MatchString(role) || len(c.Rules) != 1 {
		return fault(badRequestCode)
	}
	destinations := c.Rules[0].Destinations
	if len(destinations) < 1 || len(destinations) > 5 {
		return fault(badRequestCode)
	}
	for _, d := range destinations {
		if !bucketArnPattern.MatchString(aws.ToString(d.DestinationTableBucketARN)) {
			return fault(badRequestCode)
		}
	}
	return nil
}

func (f *fakeAPI) GetTableReplication(ctx context.Context, in *s3tables.GetTableReplicationInput, _ ...func(*s3tables.Options)) (*s3tables.GetTableReplicationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetTableReplication"); err != nil {
		return nil, err
	}
	arn := aws.ToString(in.TableArn)
	if err := table(arn); err != nil {
		return nil, err
	}
	stored := f.store.Resource(tableReplicationType, arn)
	if stored == nil {
		return nil, fault(notFoundCode)
	}
	configuration, err := configurationOf(stored)
	if err != nil {
		return nil, err
	}
	return &s3tables.GetTableReplicationOutput{Configuration: configuration, VersionToken: aws.String(f.version(arn))}, nil
}

func (f *fakeAPI) PutTableReplication(ctx context.Context, in *s3tables.PutTableReplicationInput, _ ...func(*s3tables.Options)) (*s3tables.PutTableReplicationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PutTableReplication"); err != nil {
		return nil, err
	}
	arn := aws.ToString(in.TableArn)
	if err := table(arn); err != nil {
		return nil, err
	}
	if err := validConfiguration(in.Configuration); err != nil {
		return nil, err
	}
	if in.VersionToken != nil {
		if err := f.checkVersion(arn, in.VersionToken); err != nil {
			return nil, err
		}
	}
	configuration, err := awsapi.EncodeObject(in.Configuration)
	if err != nil {
		return nil, err
	}
	f.store.Put(tableReplicationType, arn, map[string]any{"TableArn": arn, "Configuration": configuration})
	f.nextVersion(arn)
	return &s3tables.PutTableReplicationOutput{Status: aws.String("ACTIVE"), VersionToken: aws.String(f.version(arn))}, nil
}

func (f *fakeAPI) DeleteTableReplication(ctx context.Context, in *s3tables.DeleteTableReplicationInput, _ ...func(*s3tables.Options)) (*s3tables.DeleteTableReplicationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteTableReplication"); err != nil {
		return nil, err
	}
	arn := aws.ToString(in.TableArn)
	if err := table(arn); err != nil {
		return nil, err
	}
	if in.VersionToken == nil {
		return nil, fault(badRequestCode)
	}
	if f.store.Resource(tableReplicationType, arn) == nil {
		return nil, fault(notFoundCode)
	}
	if err := f.checkVersion(arn, in.VersionToken); err != nil {
		return nil, err
	}
	f.store.Remove(tableReplicationType, arn)
	f.nextVersion(arn)
	return &s3tables.DeleteTableReplicationOutput{}, nil
}
