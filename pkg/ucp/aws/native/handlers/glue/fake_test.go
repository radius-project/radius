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

package glue

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	// account is the account of the fake's caller, whose Data Catalog a call that leaves out CatalogId uses.
	account = "123456789012"

	// maxIndexes is the most partition indexes a table can have.
	maxIndexes = 3

	// pageSize is how many indexes a GetPartitionIndexes page has. AWS doesn't document its page size.
	pageSize = 2

	// validationCode is the error the Common Errors page gives for input that doesn't meet a documented constraint,
	// such as a length or a pattern.
	validationCode = "ValidationError"

	// invalidInputCode is the error each call lists for input that isn't valid, which the fake returns for a rule
	// the docs state in words.
	invalidInputCode = "InvalidInputException"
)

// table is a table in the fake's Data Catalog: its partition keys, and whether its partitions are encrypted, which
// makes an index on it fail.
type table struct {
	keys      []types.Column
	encrypted bool
}

func column(name string, typ string) types.Column {
	return types.Column{Name: aws.String(name), Type: aws.String(typ)}
}

// tables are the tables in the caller's Data Catalog, by database|table.
var tables = map[string]table{
	"sales|orders": {keys: []types.Column{
		column("year", "int"), column("month", "int"), column("region", "string"), column("day", "date"),
		column("label", "varchar(20)"), column("attributes", "map<string,string>"),
	}},
	"sales|events": {keys: []types.Column{column("year", "int"), column("kind", "string")}},
	"sales|secret": {keys: []types.Column{column("year", "int")}, encrypted: true},
}

// indexableTypes are the column types a partition index can cover. char and varchar take a length.
var indexableTypes = regexp.MustCompile(`^(string|int|bigint|long|tinyint|smallint|date|(char|varchar)\(\d+\))$`)

// namePattern is the documented pattern of Glue names and IDs.
var namePattern = regexp.MustCompile(`^[\x{20}-\x{D7FF}\x{E000}-\x{FFFD}\x{10000}-\x{10FFFF}\t]*$`)

// fakeAPI is an in-memory AWS Glue API for partition indexes. It keeps them in an awsfake store, in the type's
// property format, so the suite's Cloud Control sees the same resources. A new index reports CREATING for one read
// that returns it before it is ACTIVE, or FAILED on a table with encrypted partitions, and a deleted one reports
// DELETING for one read that returns it before it is gone. It follows the rules in partitionIndexAPI.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu     sync.Mutex
	status map[string]types.PartitionIndexStatus

	// deletes counts the DeletePartitionIndex calls that reached the fake.
	deletes int

	// holdCreating keeps a new index CREATING, as for a table with many partitions.
	holdCreating bool

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, status: map[string]types.PartitionIndexStatus{}, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// validName reports whether v is 1 to 255 characters of the documented pattern.
func validName(v string) bool {
	n := utf8.RuneCountInString(v)
	return n >= 1 && n <= 255 && namePattern.MatchString(v)
}

// validNames returns ValidationError unless each name is valid. A CatalogId the call leaves out is nil.
func validNames(catalog *string, names ...*string) error {
	if catalog != nil && !validName(*catalog) {
		return fault(validationCode)
	}
	for _, n := range names {
		if n == nil || !validName(*n) {
			return fault(validationCode)
		}
	}
	return nil
}

// tableOf returns the table the call names, with the database and table names folded to lowercase, as Glue stores
// them, or EntityNotFoundException.
func tableOf(catalog *string, database *string, name *string) (string, table, error) {
	c := aws.ToString(catalog)
	if c == "" {
		c = account
	}
	key := strings.ToLower(aws.ToString(database)) + "|" + strings.ToLower(aws.ToString(name))
	t, ok := tables[key]
	if c != account || !ok {
		return "", table{}, fault("EntityNotFoundException")
	}
	return c + "|" + key, t, nil
}

// indexes returns the identifiers of the indexes on the table prefix names, in the order the store lists them.
func (f *fakeAPI) indexes(prefix string) []string {
	out := []string{}
	for _, id := range f.store.List(partitionIndexType) {
		if strings.HasPrefix(id, prefix+"|") {
			out = append(out, id)
		}
	}
	return out
}

// statusOf returns the index's status: ACTIVE unless the fake keeps another for it.
func (f *fakeAPI) statusOf(id string) types.PartitionIndexStatus {
	if status, ok := f.status[id]; ok {
		return status
	}
	return types.PartitionIndexStatusActive
}

// reported moves the status of an index a read returned on: a create finishes, ACTIVE, or FAILED on a table with
// encrypted partitions, and a delete removes the index. An index stays CREATING while holdCreating is set.
func (f *fakeAPI) reported(id string, t table) {
	switch f.status[id] {
	case types.PartitionIndexStatusCreating:
		switch {
		case f.holdCreating:
		case t.encrypted:
			f.status[id] = types.PartitionIndexStatusFailed
		default:
			delete(f.status, id)
		}
	case types.PartitionIndexStatusDeleting:
		delete(f.status, id)
		f.store.Remove(partitionIndexType, id)
	}
}

func (f *fakeAPI) change(apply func() error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

func (f *fakeAPI) CreatePartitionIndex(ctx context.Context, in *glue.CreatePartitionIndexInput, _ ...func(*glue.Options)) (*glue.CreatePartitionIndexOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["CreatePartitionIndex"]; err != nil {
		return nil, err
	}
	if in.PartitionIndex == nil {
		return nil, fault(validationCode)
	}
	index := in.PartitionIndex
	if err := validNames(in.CatalogId, in.DatabaseName, in.TableName, index.IndexName); err != nil {
		return nil, err
	}
	if len(index.Keys) == 0 || slices.ContainsFunc(index.Keys, func(k string) bool { return !validName(k) }) {
		return nil, fault(validationCode)
	}
	prefix, t, err := tableOf(in.CatalogId, in.DatabaseName, in.TableName)
	if err != nil {
		return nil, err
	}
	for _, key := range index.Keys {
		i := slices.IndexFunc(t.keys, func(c types.Column) bool { return aws.ToString(c.Name) == key })
		if i < 0 || !indexableTypes.MatchString(aws.ToString(t.keys[i].Type)) {
			return nil, fault(invalidInputCode)
		}
	}
	id := prefix + "|" + aws.ToString(index.IndexName)
	existing := f.indexes(prefix)
	if slices.Contains(existing, id) {
		return nil, fault("AlreadyExistsException")
	}
	counted := slices.DeleteFunc(existing, func(i string) bool { return f.status[i] == types.PartitionIndexStatusFailed })
	if len(counted) >= maxIndexes {
		return nil, fault("ResourceNumberLimitExceededException")
	}
	return &glue.CreatePartitionIndexOutput{}, f.change(func() error {
		parts := strings.Split(prefix, "|")
		f.store.Put(partitionIndexType, id, map[string]any{
			"CatalogId":      parts[0],
			"DatabaseName":   parts[1],
			"TableName":      parts[2],
			"PartitionIndex": map[string]any{"IndexName": aws.ToString(index.IndexName), "Keys": anyList(index.Keys)},
		})
		f.status[id] = types.PartitionIndexStatusCreating
		return nil
	})
}

func anyList(list []string) []any {
	out := []any{}
	for _, v := range list {
		out = append(out, v)
	}
	return out
}

// GetPartitionIndexes returns pageSize indexes per page.
func (f *fakeAPI) GetPartitionIndexes(ctx context.Context, in *glue.GetPartitionIndexesInput, _ ...func(*glue.Options)) (*glue.GetPartitionIndexesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["GetPartitionIndexes"]; err != nil {
		return nil, err
	}
	if err := validNames(in.CatalogId, in.DatabaseName, in.TableName); err != nil {
		return nil, err
	}
	prefix, t, err := tableOf(in.CatalogId, in.DatabaseName, in.TableName)
	if err != nil {
		return nil, err
	}
	ids := f.indexes(prefix)
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	end := min(start+pageSize, len(ids))
	out := &glue.GetPartitionIndexesOutput{PartitionIndexDescriptorList: []types.PartitionIndexDescriptor{}}
	for _, id := range ids[min(start, end):end] {
		status := f.statusOf(id)
		index, _ := f.store.Resource(partitionIndexType, id)["PartitionIndex"].(map[string]any)
		d := types.PartitionIndexDescriptor{IndexName: aws.String(index["IndexName"].(string)), IndexStatus: status}
		keys, _ := index["Keys"].([]any)
		for _, k := range keys {
			i := slices.IndexFunc(t.keys, func(c types.Column) bool { return aws.ToString(c.Name) == k })
			d.Keys = append(d.Keys, types.KeySchemaElement{Name: aws.String(k.(string)), Type: t.keys[i].Type})
		}
		if status == types.PartitionIndexStatusFailed {
			d.BackfillErrors = []types.BackfillError{{Code: types.BackfillErrorCodeEncryptedPartitionError}}
		}
		out.PartitionIndexDescriptorList = append(out.PartitionIndexDescriptorList, d)
		f.reported(id, t)
	}
	if end < len(ids) {
		out.NextToken = aws.String(strconv.Itoa(end))
	}
	return out, nil
}

func (f *fakeAPI) DeletePartitionIndex(ctx context.Context, in *glue.DeletePartitionIndexInput, _ ...func(*glue.Options)) (*glue.DeletePartitionIndexOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	if err := f.errs["DeletePartitionIndex"]; err != nil {
		return nil, err
	}
	if err := validNames(in.CatalogId, in.DatabaseName, in.TableName, in.IndexName); err != nil {
		return nil, err
	}
	prefix, _, err := tableOf(in.CatalogId, in.DatabaseName, in.TableName)
	if err != nil {
		return nil, err
	}
	id := prefix + "|" + aws.ToString(in.IndexName)
	if f.store.Resource(partitionIndexType, id) == nil || f.status[id] == types.PartitionIndexStatusDeleting {
		return nil, fault("EntityNotFoundException")
	}
	if f.status[id] == types.PartitionIndexStatusCreating {
		return nil, fault(invalidInputCode)
	}
	return &glue.DeletePartitionIndexOutput{}, f.change(func() error {
		f.status[id] = types.PartitionIndexStatusDeleting
		return nil
	})
}

// deleteDirect deletes the index name on sales.orders, as a delete UCP lost the response of did.
func (f *fakeAPI) deleteDirect(name string) error {
	_, err := f.DeletePartitionIndex(context.Background(), &glue.DeletePartitionIndexInput{
		DatabaseName: aws.String("sales"), TableName: aws.String("orders"), IndexName: aws.String(name),
	})
	return err
}
