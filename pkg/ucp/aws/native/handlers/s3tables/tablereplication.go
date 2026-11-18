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

// Package s3tables has the native handler for the replication configuration of an S3 table
// (AWS::S3Tables::TableReplication), which has no CloudFormation type. Every call takes effect at once, so the handler
// never waits for a state.
package s3tables

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3tables"
	"github.com/aws/aws-sdk-go-v2/service/s3tables/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	tableReplicationType = "AWS::S3Tables::TableReplication"

	// notFoundCode is the error S3 Tables returns for a table, or a replication configuration, it doesn't have.
	notFoundCode = "NotFoundException"
)

// API is the part of the S3 Tables client the handler uses.
type API interface {
	GetTableReplication(ctx context.Context, params *s3tables.GetTableReplicationInput, optFns ...func(*s3tables.Options)) (*s3tables.GetTableReplicationOutput, error)
	PutTableReplication(ctx context.Context, params *s3tables.PutTableReplicationInput, optFns ...func(*s3tables.Options)) (*s3tables.PutTableReplicationOutput, error)
	DeleteTableReplication(ctx context.Context, params *s3tables.DeleteTableReplicationInput, optFns ...func(*s3tables.Options)) (*s3tables.DeleteTableReplicationOutput, error)
}

var inRegion = awsapi.InRegion[s3tables.Options]

var _ native.Handler = (*TableReplication)(nil)

// TableReplication serves AWS::S3Tables::TableReplication, the replication configuration of one table. Its identifier
// is the table's ARN, TableArn. PutTableReplication replaces the whole configuration, so a create sent again sets
// the same one. An update and a delete send the version token of the configuration they read, so they never change
// a configuration someone else changed since.
type TableReplication struct {
	api API
}

// NewTableReplication returns the handler for the replication configuration of a table.
func NewTableReplication(api API) *TableReplication {
	return &TableReplication{api: api}
}

// TypeName returns AWS::S3Tables::TableReplication.
func (h *TableReplication) TypeName() string {
	return tableReplicationType
}

// Options returns how UCP uses the handler. Replication configurations can't be listed.
func (h *TableReplication) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// get returns the table's replication configuration and its version token, or ErrNotFound when the table has none.
func (h *TableReplication) get(ctx context.Context, region string, table string) (*s3tables.GetTableReplicationOutput, error) {
	out, err := h.api.GetTableReplication(ctx, &s3tables.GetTableReplicationInput{TableArn: aws.String(table)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, notFoundCode)
	}
	return out, nil
}

// Read returns the table's replication configuration, or ErrNotFound when the table has none.
func (h *TableReplication) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	out, err := h.get(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	properties := map[string]any{"TableArn": identifier}
	if out.Configuration != nil {
		configuration, err := awsapi.EncodeObject(out.Configuration)
		if err != nil {
			return nil, err
		}
		properties["Configuration"] = configuration
	}
	return properties, nil
}

// configurationOf returns the Configuration properties set, or a client failure when they set none.
func configurationOf(properties map[string]any) (*types.TableReplicationConfiguration, error) {
	value, ok := properties["Configuration"].(map[string]any)
	if !ok {
		return nil, &native.Failure{Message: "set Configuration", ClientError: true, Outcome: native.OutcomeRejected}
	}
	out := &types.TableReplicationConfiguration{}
	if err := awsapi.Decode(value, out); err != nil {
		return nil, &native.Failure{Message: "Configuration doesn't have the form S3 Tables takes: " + err.Error(), ClientError: true, Outcome: native.OutcomeRejected}
	}
	return out, nil
}

func (h *TableReplication) put(ctx context.Context, region string, table string, properties map[string]any, version *string) error {
	configuration, err := configurationOf(properties)
	if err != nil {
		return err
	}
	_, err = h.api.PutTableReplication(ctx, &s3tables.PutTableReplicationInput{
		TableArn:      aws.String(table),
		Configuration: configuration,
		VersionToken:  version,
	}, inRegion(region))
	return err
}

// Create sets the table's replication configuration and returns the table's ARN. UCP calls it only when the table has
// no configuration, so it sends no version token.
func (h *TableReplication) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	table := awsapi.String(request.Properties, "TableArn")
	if err := h.put(ctx, request.Region, table, request.Properties, nil); err != nil {
		return "", err
	}
	return table, nil
}

// Update reads the configuration's version token and replaces the configuration with the template's.
func (h *TableReplication) Update(ctx context.Context, request native.UpdateRequest) error {
	if _, err := configurationOf(request.Desired); err != nil {
		return err
	}
	current, err := h.get(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	return h.put(ctx, request.Region, request.Identifier, request.Desired, current.VersionToken)
}

// Delete reads the configuration's version token and deletes the configuration. It returns ErrNotFound when the
// table has none. Replicas already made stay in their table buckets.
func (h *TableReplication) Delete(ctx context.Context, region string, identifier string) error {
	current, err := h.get(ctx, region, identifier)
	if err != nil {
		return err
	}
	_, err = h.api.DeleteTableReplication(ctx, &s3tables.DeleteTableReplicationInput{
		TableArn:     aws.String(identifier),
		VersionToken: current.VersionToken,
	}, inRegion(region))
	return awsapi.NotFound(err, notFoundCode)
}

// List fails, because replication configurations can't be listed.
func (h *TableReplication) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", tableReplicationType)
}

// Progress reports every operation as done, because S3 Tables applies a replication configuration at once. Replicating
// the table's data runs on after that.
func (h *TableReplication) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
