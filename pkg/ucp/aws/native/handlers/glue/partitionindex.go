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

// Package glue has the native handler for AWS Glue partition indexes, which have no CloudFormation type.
package glue

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/glue/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const partitionIndexType = "AWS::Glue::PartitionIndex"

// API is the part of the AWS Glue client the handler uses.
type API interface {
	CreatePartitionIndex(ctx context.Context, params *glue.CreatePartitionIndexInput, optFns ...func(*glue.Options)) (*glue.CreatePartitionIndexOutput, error)
	GetPartitionIndexes(ctx context.Context, params *glue.GetPartitionIndexesInput, optFns ...func(*glue.Options)) (*glue.GetPartitionIndexesOutput, error)
	DeletePartitionIndex(ctx context.Context, params *glue.DeletePartitionIndexInput, optFns ...func(*glue.Options)) (*glue.DeletePartitionIndexOutput, error)
}

var inRegion = awsapi.InRegion[glue.Options]

var (
	_ native.Handler            = (*PartitionIndex)(nil)
	_ native.IdentifierEncoder  = (*PartitionIndex)(nil)
	_ native.IdentifierResolver = (*PartitionIndex)(nil)
)

// createStates are the states a create waits for. AWS builds a new index from the table's partitions, and the index
// fails when a partition can't be indexed.
var createStates = awsapi.States{Done: []string{string(types.PartitionIndexStatusActive)}, Failed: []string{string(types.PartitionIndexStatusFailed)}}

// PartitionIndex serves AWS::Glue::PartitionIndex: an index on some of a Glue table's partition keys. Its identifier is
// CatalogId, DatabaseName, TableName, and the index's IndexName, joined with "|". Glue stores database and table names
// in lowercase, so the identifier has them in lowercase. AWS has no call that changes an index, so every property is
// create-only, and UCP rejects a change with 400 before any AWS call.
type PartitionIndex struct {
	api API
}

// NewPartitionIndex returns the handler for Glue partition indexes.
func NewPartitionIndex(api API) *PartitionIndex {
	return &PartitionIndex{api: api}
}

// TypeName returns AWS::Glue::PartitionIndex.
func (h *PartitionIndex) TypeName() string {
	return partitionIndexType
}

// Options returns how UCP uses the handler. AWS lists indexes only by table, so the type can't be listed.
func (h *PartitionIndex) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// indexID names a partition index.
type indexID struct {
	catalog, database, table, name string
}

func (id indexID) String() string {
	return strings.Join([]string{id.catalog, id.database, id.table, id.name}, "|")
}

// badName is the failure for a name with "|", which separates the names in the identifier.
var badName = &native.Failure{
	Message:     "CatalogId, DatabaseName, TableName, and PartitionIndex.IndexName of an AWS::Glue::PartitionIndex can't contain \"|\", which Radius uses to join them into the index's identifier",
	ClientError: true,
	Outcome:     native.OutcomeRejected,
}

// wrongType is the failure for a property whose value has the wrong type, so it is never sent as an empty or shorter
// value.
func wrongType(path string, want string) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s of an AWS::Glue::PartitionIndex must be %s", path, want),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// stringOf returns the string property name of properties, or "" when it is absent.
func stringOf(properties map[string]any, name string, path string) (string, error) {
	value, ok := properties[name]
	if !ok || value == nil {
		return "", nil
	}
	s, ok := value.(string)
	if !ok {
		return "", wrongType(path, "a string")
	}
	return s, nil
}

// keysOf returns the partition keys of the index, or nil when it has none.
func keysOf(index map[string]any) ([]string, error) {
	value, ok := index["Keys"]
	if !ok || value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	keys := []string{}
	for _, v := range list {
		key, isString := v.(string)
		ok = ok && isString
		keys = append(keys, key)
	}
	if !ok {
		return nil, wrongType("PartitionIndex.Keys", "a list of partition key names")
	}
	return keys, nil
}

// parse returns the index the properties name, with the database and table names in lowercase, and its keys. ok is
// false when they don't name all of it.
func parse(properties map[string]any) (id indexID, keys []string, ok bool, err error) {
	var index map[string]any
	if value := properties["PartitionIndex"]; value != nil {
		if index, ok = value.(map[string]any); !ok {
			return indexID{}, nil, false, wrongType("PartitionIndex", "an object with IndexName and Keys")
		}
	}
	for _, field := range []struct {
		properties map[string]any
		name, path string
		value      *string
	}{
		{properties, "CatalogId", "CatalogId", &id.catalog},
		{properties, "DatabaseName", "DatabaseName", &id.database},
		{properties, "TableName", "TableName", &id.table},
		{index, "IndexName", "PartitionIndex.IndexName", &id.name},
	} {
		if *field.value, err = stringOf(field.properties, field.name, field.path); err != nil {
			return indexID{}, nil, false, err
		}
	}
	if keys, err = keysOf(index); err != nil {
		return indexID{}, nil, false, err
	}
	id.database, id.table = strings.ToLower(id.database), strings.ToLower(id.table)
	parts := []string{id.catalog, id.database, id.table, id.name}
	if slices.ContainsFunc(parts, func(v string) bool { return strings.Contains(v, "|") }) {
		return indexID{}, nil, false, badName
	}
	return id, keys, !slices.Contains(parts, ""), nil
}

// parseID returns the index an identifier names.
func parseID(identifier string) (indexID, error) {
	parts := strings.Split(identifier, "|")
	if len(parts) != 4 || slices.Contains(parts, "") {
		return indexID{}, &native.Failure{
			Message:     fmt.Sprintf("%q is not the identifier of an AWS Glue partition index: it must be CatalogId|DatabaseName|TableName|IndexName", identifier),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return indexID{catalog: parts[0], database: strings.ToLower(parts[1]), table: strings.ToLower(parts[2]), name: parts[3]}, nil
}

// Identifier returns CatalogId|DatabaseName|TableName|IndexName, with the database and table names in lowercase, or ""
// when the properties don't give all four. UCP calls it before any AWS call, so a value of the wrong type, such as a
// key that isn't a string, is rejected here.
func (h *PartitionIndex) Identifier(properties map[string]any) (string, error) {
	id, _, ok, err := parse(properties)
	if !ok || err != nil {
		return "", err
	}
	return id.String(), nil
}

// ResolveIdentifier returns the identifier with the database and table names in lowercase, so a name that gives them
// in another case names the same index.
func (h *PartitionIndex) ResolveIdentifier(ctx context.Context, region string, identifier string) (string, error) {
	id, err := parseID(identifier)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// describe returns the index, or ErrNotFound when the table, or the index on it, doesn't exist.
func (h *PartitionIndex) describe(ctx context.Context, region string, id indexID) (*types.PartitionIndexDescriptor, error) {
	input := &glue.GetPartitionIndexesInput{CatalogId: aws.String(id.catalog), DatabaseName: aws.String(id.database), TableName: aws.String(id.table)}
	for {
		page, err := h.api.GetPartitionIndexes(ctx, input, inRegion(region))
		if err != nil {
			return nil, awsapi.NotFound(err, "EntityNotFoundException")
		}
		for i := range page.PartitionIndexDescriptorList {
			if index := &page.PartitionIndexDescriptorList[i]; aws.ToString(index.IndexName) == id.name {
				return index, nil
			}
		}
		if input.NextToken = page.NextToken; input.NextToken == nil {
			return nil, native.ErrNotFound
		}
	}
}

// Read returns the index's properties. An index AWS failed to build is a client failure, so a deploy that finds one,
// such as a redeploy after the create failed, or a take-over, fails instead of reporting an index that doesn't work.
// A delete finds the index without Read, so it still deletes it.
func (h *PartitionIndex) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, err := parseID(identifier)
	if err != nil {
		return nil, err
	}
	index, err := h.describe(ctx, region, id)
	if err != nil {
		return nil, err
	}
	if index.IndexStatus == types.PartitionIndexStatusFailed {
		return nil, &native.Failure{Message: backfillFailure(id.name, index.BackfillErrors), ClientError: true, Outcome: native.OutcomeRejected}
	}
	keys := []any{}
	for _, k := range index.Keys {
		keys = append(keys, aws.ToString(k.Name))
	}
	return map[string]any{
		"CatalogId":      id.catalog,
		"DatabaseName":   id.database,
		"TableName":      id.table,
		"PartitionIndex": map[string]any{"IndexName": id.name, "Keys": keys},
		"IndexStatus":    string(index.IndexStatus),
	}, nil
}

// Create starts creating the index. AWS builds it from the table's partitions in the background.
func (h *PartitionIndex) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	id, keys, ok, err := parse(request.Properties)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", &native.Failure{
			Message:     "an AWS::Glue::PartitionIndex needs CatalogId, DatabaseName, TableName, and PartitionIndex.IndexName",
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	_, err = h.api.CreatePartitionIndex(ctx, &glue.CreatePartitionIndexInput{
		CatalogId:      aws.String(id.catalog),
		DatabaseName:   aws.String(id.database),
		TableName:      aws.String(id.table),
		PartitionIndex: &types.PartitionIndex{IndexName: aws.String(id.name), Keys: keys},
	}, inRegion(request.Region))
	if awsapi.HasCode(err, "AlreadyExistsException") {
		// UCP reads AWS by the identifier before it creates an index, so this index may be the one an earlier attempt
		// of this create made, such as when the SDK retried a call whose response was lost. The outcome is then
		// unknown, so UCP keeps the record and the next deploy finds the index by its identifier.
		return "", &native.Failure{
			Message:     fmt.Sprintf("the table already has a partition index named %q, which an earlier attempt of this deploy may have created. Deploy again to manage it, or choose another name: %v", id.name, err),
			ClientError: true,
			Outcome:     native.OutcomeUnknown,
		}
	}
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// Update changes nothing, because every property is create-only.
func (h *PartitionIndex) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete starts deleting the index. An index AWS is already deleting had a delete reach AWS before, so the delete
// succeeds. AWS doesn't delete an index it is still creating, and then its error is returned.
func (h *PartitionIndex) Delete(ctx context.Context, region string, identifier string) error {
	id, err := parseID(identifier)
	if err != nil {
		return err
	}
	index, err := h.describe(ctx, region, id)
	if err != nil {
		return err
	}
	if index.IndexStatus == types.PartitionIndexStatusDeleting {
		return nil
	}
	_, err = h.api.DeletePartitionIndex(ctx, &glue.DeletePartitionIndexInput{
		CatalogId:    aws.String(id.catalog),
		DatabaseName: aws.String(id.database),
		TableName:    aws.String(id.table),
		IndexName:    aws.String(id.name),
	}, inRegion(region))
	return awsapi.NotFound(err, "EntityNotFoundException")
}

// List returns nothing, because AWS lists partition indexes only by table.
func (h *PartitionIndex) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, nil
}

// Progress reads the index's status. A create waits for ACTIVE, and fails at FAILED, naming AWS's backfill error codes.
// A delete waits for the index to be gone. An update changes nothing, so it is done.
func (h *PartitionIndex) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	id, err := parseID(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	index, err := h.describe(ctx, region, id)
	if err != nil {
		return native.Progress{}, err
	}
	status := string(index.IndexStatus)
	if op == native.OperationDelete {
		return native.Progress{State: status, Waiting: "deleted"}, nil
	}
	progress := createStates.Progress(status)
	if progress.Failure != nil {
		progress.Failure.Message = backfillFailure(id.name, index.BackfillErrors)
		progress.Failure.ClientError = true
	}
	return progress, nil
}

// backfillFailure returns the message of an index AWS failed to build, naming each error code once.
func backfillFailure(name string, errs []types.BackfillError) string {
	codes := []string{}
	for _, e := range errs {
		if code := string(e.Code); code != "" && !slices.Contains(codes, code) {
			codes = append(codes, code)
		}
	}
	message := fmt.Sprintf("AWS Glue failed to build the partition index %q from the table's partitions", name)
	if len(codes) > 0 {
		message += " (" + strings.Join(codes, ", ") + ")"
	}
	return message + ". Fix the partitions, then delete the index and deploy it again"
}
