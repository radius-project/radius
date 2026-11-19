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
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/glue/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/glue/latest/webapi/"
	commonErrors = apiReference + "CommonErrors.html"
	indexGuide   = "https://docs.aws.amazon.com/glue/latest/dg/partition-indexes.html"
	tableInput   = apiReference + "API_TableInput.html"
	databaseDoc  = apiReference + "API_DatabaseInput.html"
	descriptor   = apiReference + "API_PartitionIndexDescriptor.html"

	// constraintNote is what every call's page leaves unclear about its constraints.
	constraintNote = " The page names no error for a value outside its length or pattern; the fake returns " +
		"ValidationError, from the Common Errors page."
)

// apiCall is a call against a fake Glue API, after any calls it needs to set up.
type apiCall func(ctx context.Context, f *fakeAPI) error

// against returns calls as a rule's Break or Check, made in order against one fake Glue API backed by store. It
// stops at the first call that fails, and returns its error.
func against(calls ...apiCall) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		f := newFakeAPI(store)
		for _, call := range calls {
			if err := call(ctx, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// createIndex creates the index by_date on sales.orders over year and month, with the changes edits make to the
// input.
func createIndex(edits ...func(*glue.CreatePartitionIndexInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &glue.CreatePartitionIndexInput{
			CatalogId:      aws.String(account),
			DatabaseName:   aws.String("sales"),
			TableName:      aws.String("orders"),
			PartitionIndex: &types.PartitionIndex{IndexName: aws.String("by_date"), Keys: []string{"year", "month"}},
		}
		for _, edit := range edits {
			edit(in)
		}
		_, err := f.CreatePartitionIndex(ctx, in)
		return err
	}
}

// named returns an edit that names the index name.
func named(name string) func(*glue.CreatePartitionIndexInput) {
	return func(in *glue.CreatePartitionIndexInput) { in.PartitionIndex.IndexName = aws.String(name) }
}

// keyed returns an edit that sets the index's keys.
func keyed(keys ...string) func(*glue.CreatePartitionIndexInput) {
	return func(in *glue.CreatePartitionIndexInput) { in.PartitionIndex.Keys = keys }
}

// getIndexes returns the indexes on the table, from every page.
func getIndexes(ctx context.Context, f *fakeAPI, in *glue.GetPartitionIndexesInput) ([]types.PartitionIndexDescriptor, error) {
	out := []types.PartitionIndexDescriptor{}
	for {
		page, err := f.GetPartitionIndexes(ctx, in)
		if err != nil {
			return nil, err
		}
		out = append(out, page.PartitionIndexDescriptorList...)
		if in.NextToken = page.NextToken; in.NextToken == nil {
			return out, nil
		}
	}
}

// statusOf returns the status of the index name on sales.orders in the caller's catalog, or "" when it is gone.
func statusOf(ctx context.Context, f *fakeAPI, name string) (types.PartitionIndexStatus, error) {
	indexes, err := getIndexes(ctx, f, &glue.GetPartitionIndexesInput{DatabaseName: aws.String("sales"), TableName: aws.String("orders")})
	if err != nil {
		return "", err
	}
	for _, i := range indexes {
		if aws.ToString(i.IndexName) == name {
			return i.IndexStatus, nil
		}
	}
	return "", nil
}

func deleteIndex(edit func(*glue.DeletePartitionIndexInput)) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		in := &glue.DeletePartitionIndexInput{
			CatalogId:    aws.String(account),
			DatabaseName: aws.String("sales"),
			TableName:    aws.String("orders"),
			IndexName:    aws.String("by_date"),
		}
		if edit != nil {
			edit(in)
		}
		_, err := f.DeletePartitionIndex(ctx, in)
		return err
	}
}

// settle reads sales.orders until no index on it is CREATING.
func settle(ctx context.Context, f *fakeAPI) error {
	return settle2("orders")(ctx, f)
}

// settle2 returns a call that reads the table name in the sales database twice, so a new index on it finishes.
func settle2(name string) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		for range 2 {
			if _, err := getIndexes(ctx, f, &glue.GetPartitionIndexesInput{DatabaseName: aws.String("sales"), TableName: aws.String(name)}); err != nil {
				return err
			}
		}
		return nil
	}
}

// createSecret returns a call that creates the index name on sales.secret, whose encrypted partitions make it fail.
func createSecret(name string) apiCall {
	return createIndex(named(name), func(c *glue.CreatePartitionIndexInput) {
		c.TableName = aws.String("secret")
		c.PartitionIndex.Keys = []string{"year"}
	})
}

// onActiveIndex returns a call that creates the index by_date on sales.orders and reads it until it is ACTIVE, then
// calls call.
func onActiveIndex(call apiCall) apiCall {
	return func(ctx context.Context, f *fakeAPI) error {
		if err := createIndex()(ctx, f); err != nil {
			return err
		}
		for range 2 {
			if _, err := statusOf(ctx, f, "by_date"); err != nil {
				return err
			}
		}
		return call(ctx, f)
	}
}

// expect returns an error unless got is want.
func expect[T comparable](what string, got T, want T) error {
	if got != want {
		return fmt.Errorf("%s is %v, not %v", what, got, want)
	}
	return nil
}

// partitionIndexAPI records the Glue calls PartitionIndex makes.
var partitionIndexAPI = map[string]nativetest.Call{
	"CreatePartitionIndex": {
		Doc: apiReference + "API_CreatePartitionIndex.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "IndexName is 1 to 255 characters",
				Doc:   apiReference + "API_PartitionIndex.html",
				Break: against(createIndex(named(strings.Repeat("a", 256)))),
				Code:  validationCode,
			},
			{
				Rule:  "names are characters of the documented pattern, which leaves out control characters but tab",
				Doc:   commonErrors,
				Break: against(createIndex(func(in *glue.CreatePartitionIndexInput) { in.DatabaseName = aws.String("sales\x01") })),
				Code:  validationCode,
			},
			{
				Rule: "the 255-character limit counts characters, not bytes",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					return createIndex(named(strings.Repeat("界", 255)))(ctx, f)
				}),
			},
			{
				Rule:  "Keys has at least 1 item",
				Doc:   apiReference + "API_PartitionIndex.html",
				Break: against(createIndex(keyed())),
				Code:  validationCode,
			},
			{
				Rule:  "the table exists",
				Break: against(createIndex(func(in *glue.CreatePartitionIndexInput) { in.TableName = aws.String("missing") })),
				Code:  "EntityNotFoundException",
			},
			{
				Rule: "CatalogId defaults to the caller's account",
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					if err := createIndex(func(in *glue.CreatePartitionIndexInput) { in.CatalogId = nil })(ctx, f); err != nil {
						return err
					}
					indexes, err := getIndexes(ctx, f, &glue.GetPartitionIndexesInput{CatalogId: aws.String(account), DatabaseName: aws.String("sales"), TableName: aws.String("orders")})
					if err != nil {
						return err
					}
					return expect("the number of indexes in the account's catalog", len(indexes), 1)
				}),
			},
			{
				Rule: "Glue folds database and table names to lowercase when it stores them",
				Doc:  tableInput,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					if err := createIndex(func(in *glue.CreatePartitionIndexInput) {
						in.DatabaseName, in.TableName = aws.String("Sales"), aws.String("ORDERS")
					})(ctx, f); err != nil {
						return err
					}
					status, err := statusOf(ctx, f, "by_date")
					if err != nil {
						return err
					}
					return expect("the status of the index on sales.orders", status, types.PartitionIndexStatusCreating)
				}),
			},
			{
				Rule:  "the table has no index with the name",
				Break: against(createIndex(), createIndex()),
				Code:  "AlreadyExistsException",
			},
			{
				Rule: "a table has at most 3 partition indexes",
				Doc:  indexGuide,
				Break: against(createIndex(named("a"), keyed("year")), settle, createIndex(named("b"), keyed("month")), settle,
					createIndex(named("c"), keyed("region")), settle, createIndex(named("d"), keyed("day"))),
				Code: "ResourceNumberLimitExceededException",
			},
			{
				Rule:  "a table builds one index at a time: a create while another index is CREATING fails",
				Break: against(createIndex(named("a"), keyed("year")), createIndex(named("b"), keyed("month"))),
				Code:  "ResourceNumberLimitExceededException",
			},
			{
				Rule: "a create while another index is DELETING fails too",
				Break: against(onActiveIndex(func(ctx context.Context, f *fakeAPI) error {
					if err := deleteIndex(nil)(ctx, f); err != nil {
						return err
					}
					return createIndex(named("b"), keyed("month"))(ctx, f)
				})),
				Code: "ResourceNumberLimitExceededException",
			},
			{
				Rule: "an index AWS failed to build doesn't block a create, and its name can be used again",
				Check: against(createSecret("f"), settle2("secret"), createSecret("f"), func(ctx context.Context, f *fakeAPI) error {
					indexes, err := getIndexes(ctx, f, &glue.GetPartitionIndexesInput{DatabaseName: aws.String("sales"), TableName: aws.String("secret")})
					if err != nil {
						return err
					}
					statuses := []types.PartitionIndexStatus{}
					for _, i := range indexes {
						statuses = append(statuses, i.IndexStatus)
					}
					if !slices.Equal(statuses, []types.PartitionIndexStatus{types.PartitionIndexStatusFailed, types.PartitionIndexStatusCreating}) {
						return fmt.Errorf("the table lists %v", statuses)
					}
					return nil
				}),
			},
			{
				Rule:  "each key is a partition key of the table",
				Doc:   indexGuide,
				Break: against(createIndex(keyed("year", "price"))),
				Code:  invalidInputCode,
			},
			{
				Rule:  "each key is a string, char, varchar, int, bigint, long, tinyint, smallint, or date column",
				Doc:   indexGuide,
				Break: against(createIndex(keyed("attributes"))),
				Code:  invalidInputCode,
			},
			{
				Rule: "a char or varchar key can be indexed, and so can a date",
				Doc:  indexGuide,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					return createIndex(keyed("label", "day"))(ctx, f)
				}),
			},
			{
				Rule: "a new index is CREATING, then ACTIVE",
				Doc:  indexGuide,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					if err := createIndex()(ctx, f); err != nil {
						return err
					}
					for _, want := range []types.PartitionIndexStatus{types.PartitionIndexStatusCreating, types.PartitionIndexStatusActive} {
						status, err := statusOf(ctx, f, "by_date")
						if err != nil {
							return err
						}
						if err := expect("the status", status, want); err != nil {
							return err
						}
					}
					return nil
				}),
			},
			{
				Rule: "an index AWS can't build, such as on encrypted partitions, is FAILED, with ENCRYPTED_PARTITION_ERROR",
				Doc:  indexGuide,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					in := &glue.GetPartitionIndexesInput{DatabaseName: aws.String("sales"), TableName: aws.String("secret")}
					if err := createIndex(func(c *glue.CreatePartitionIndexInput) {
						c.TableName = in.TableName
						c.PartitionIndex.Keys = []string{"year"}
					})(ctx, f); err != nil {
						return err
					}
					for range 2 {
						indexes, err := getIndexes(ctx, f, &glue.GetPartitionIndexesInput{DatabaseName: in.DatabaseName, TableName: in.TableName})
						if err != nil {
							return err
						}
						if indexes[0].IndexStatus == types.PartitionIndexStatusFailed {
							return expect("the backfill error", indexes[0].BackfillErrors[0].Code, types.BackfillErrorCodeEncryptedPartitionError)
						}
					}
					return fmt.Errorf("the index never failed")
				}),
			},
			{
				Rule: "a table keeps its last 10 failed indexes",
				Doc:  indexGuide,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					in := &glue.GetPartitionIndexesInput{DatabaseName: aws.String("sales"), TableName: aws.String("secret")}
					for i := range 11 {
						if err := createIndex(named("f"+strconv.Itoa(i)), func(c *glue.CreatePartitionIndexInput) {
							c.TableName = in.TableName
							c.PartitionIndex.Keys = []string{"year"}
						})(ctx, f); err != nil {
							return err
						}
						if _, err := getIndexes(ctx, f, &glue.GetPartitionIndexesInput{DatabaseName: in.DatabaseName, TableName: in.TableName}); err != nil {
							return err
						}
					}
					indexes, err := getIndexes(ctx, f, in)
					if err != nil {
						return err
					}
					if len(indexes) != 10 || aws.ToString(indexes[0].IndexName) == "f0" {
						return fmt.Errorf("the table keeps %d failed indexes, starting with %s", len(indexes), aws.ToString(indexes[0].IndexName))
					}
					return nil
				}),
			},
		},
		Unclear: "The guide limits a table to 3 indexes without saying whether FAILED ones count; the fake doesn't " +
			"count them. The docs don't say a table builds one index at a time; in a run against real AWS, a create while " +
			"another index on the table was CREATING or DELETING got ResourceNumberLimitExceededException (\"Only 1 index " +
			"can be created or deleted simultaneously per table\"), and the same run could create an index with the name " +
			"of a FAILED one, which stayed listed. It says a key must be a partition key of a supported type, without naming the error, or whether " +
			"AWS checks it before the index is CREATING; the fake rejects the call with InvalidInputException, the error " +
			"the call lists for input that isn't valid. The docs don't say whether a name a FAILED index has can be used " +
			"again; the fake returns AlreadyExistsException, as for any index the table lists. They don't say whether " +
			"key names are compared ignoring case; the fake compares them as they are, and returns them as the call sent " +
			"them. The page takes one index per call and has no idempotency token, so a create sent again gets " +
			"AlreadyExistsException. The handler reads the index before it creates one, and reports that error as an " +
			"unknown outcome, because an SDK retry of a create whose response was lost gets it." + constraintNote,
	},
	"GetPartitionIndexes": {
		Doc: apiReference + "API_GetPartitionIndexes.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the table exists",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.GetPartitionIndexes(ctx, &glue.GetPartitionIndexesInput{DatabaseName: aws.String("sales"), TableName: aws.String("missing")})
					return err
				}),
				Code: "EntityNotFoundException",
			},
			{
				Rule: "TableName is 1 to 255 characters",
				Break: against(func(ctx context.Context, f *fakeAPI) error {
					_, err := f.GetPartitionIndexes(ctx, &glue.GetPartitionIndexesInput{DatabaseName: aws.String("sales"), TableName: aws.String("")})
					return err
				}),
				Code: validationCode,
			},
			{
				Rule: "each index has its keys, with their names and types, and its status",
				Doc:  descriptor,
				Check: against(onActiveIndex(func(ctx context.Context, f *fakeAPI) error {
					indexes, err := getIndexes(ctx, f, &glue.GetPartitionIndexesInput{DatabaseName: aws.String("sales"), TableName: aws.String("orders")})
					if err != nil {
						return err
					}
					if len(indexes) != 1 || len(indexes[0].Keys) != 2 {
						return fmt.Errorf("the indexes are %+v", indexes)
					}
					k := indexes[0].Keys[0]
					return expect("the first key", aws.ToString(k.Name)+" "+aws.ToString(k.Type), "year int")
				})),
			},
			{
				Rule: "a database name is folded to lowercase, so a call that names it in another case reads the same table",
				Doc:  databaseDoc,
				Check: against(onActiveIndex(func(ctx context.Context, f *fakeAPI) error {
					indexes, err := getIndexes(ctx, f, &glue.GetPartitionIndexesInput{DatabaseName: aws.String("SALES"), TableName: aws.String("Orders")})
					if err != nil {
						return err
					}
					return expect("the number of indexes", len(indexes), 1)
				})),
			},
		},
		Unclear: "The page doesn't give a page size, or say whether a name in another case reads the same table; the " +
			"fake returns 2 indexes a page, and folds names to lowercase as Glue stores them. It doesn't say how long " +
			"a deleted index stays in the list as DELETING; the fake lists it for one read." + constraintNote,
	},
	"DeletePartitionIndex": {
		Doc: apiReference + "API_DeletePartitionIndex.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "the index exists",
				Break: against(deleteIndex(nil)),
				Code:  "EntityNotFoundException",
			},
			{
				Rule:  "IndexName is 1 to 255 characters",
				Break: against(deleteIndex(func(in *glue.DeletePartitionIndexInput) { in.IndexName = aws.String(strings.Repeat("a", 256)) })),
				Code:  validationCode,
			},
			{
				Rule: "an index AWS is still creating can't be deleted",
				Doc:  descriptor,
				Check: against(func(ctx context.Context, f *fakeAPI) error {
					if err := createIndex()(ctx, f); err != nil {
						return err
					}
					if deleteIndex(nil)(ctx, f) == nil {
						return fmt.Errorf("the fake deleted an index it was creating")
					}
					return nil
				}),
			},
			{
				Rule: "an index AWS failed to build can't be deleted",
				Break: against(createSecret("f"), settle2("secret"), func(ctx context.Context, f *fakeAPI) error {
					return deleteIndex(func(in *glue.DeletePartitionIndexInput) {
						in.TableName, in.IndexName = aws.String("secret"), aws.String("f")
					})(ctx, f)
				}),
				Code: "EntityNotFoundException",
			},
			{
				Rule: "an ACTIVE index goes to DELETING, then leaves the list",
				Doc:  indexGuide,
				Check: against(onActiveIndex(func(ctx context.Context, f *fakeAPI) error {
					if err := deleteIndex(nil)(ctx, f); err != nil {
						return err
					}
					for _, want := range []types.PartitionIndexStatus{types.PartitionIndexStatusDeleting, ""} {
						status, err := statusOf(ctx, f, "by_date")
						if err != nil {
							return err
						}
						if err := expect("the status", status, want); err != nil {
							return err
						}
					}
					return nil
				})),
			},
		},
		Unclear: "The docs say an index that is CREATING can't be deleted, without naming the error; the fake returns " +
			"InvalidInputException. They don't say whether a FAILED index can be deleted; in a run against real AWS, the " +
			"call got EntityNotFoundException and the index stayed listed, so the fake does the same. They " +
			"don't say what AWS does for an index that is DELETING; the fake returns EntityNotFoundException, and the " +
			"handler doesn't call it for one." + constraintNote,
	},
}
