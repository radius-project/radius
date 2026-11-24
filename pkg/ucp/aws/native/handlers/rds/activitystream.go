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

package rds

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const activityStreamType = "AWS::RDS::DBClusterActivityStream"

var (
	_ native.Handler           = (*ActivityStream)(nil)
	_ native.IdentifierEncoder = (*ActivityStream)(nil)
	_ native.Resetter          = (*ActivityStream)(nil)
)

// ActivityStream serves AWS::RDS::DBClusterActivityStream: the database activity stream of an Aurora DB cluster. Its
// identifier is ResourceArn, the cluster's ARN. The stream is a setting of the cluster, so AWS has the resource while
// the stream is starting or started, and a stopped stream reads as absent. Every property is create-only, because AWS
// can't change a started stream: a new mode or key takes a stop and a start.
//
// The handler starts and stops the stream with ApplyImmediately, which restarts the cluster. Without it, AWS waits for
// the cluster's next maintenance window, which a deploy can't wait for. A delete stops the stream, AWS's default
// (onDelete reset), and AWS deletes the Kinesis stream it made for it.
type ActivityStream struct {
	api API
}

// NewActivityStream returns the handler for DB cluster activity streams.
func NewActivityStream(api API) *ActivityStream {
	return &ActivityStream{api: api}
}

// TypeName returns AWS::RDS::DBClusterActivityStream.
func (h *ActivityStream) TypeName() string {
	return activityStreamType
}

// Options returns how UCP uses the handler.
func (h *ActivityStream) Options() native.Options {
	return native.Options{
		List:               native.ListRegion,
		Recovery:           native.RecoverByIdentifier,
		Equivalent:         streamEquivalent,
		RecordedEquivalent: sameAlias,
	}
}

// Identifier returns ResourceArn in lowercase, as RDS stores a cluster's identifier and ARN. A value that isn't a DB
// cluster's ARN is a client failure, such as a DB instance's ARN, which StartActivityStream also takes for RDS for
// Oracle and SQL Server but this type doesn't serve.
func (h *ActivityStream) Identifier(properties map[string]any) (string, error) {
	value := awsapi.String(properties, "ResourceArn")
	if value == "" {
		return "", nil
	}
	parsed, err := arn.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Service, "rds") || !strings.HasPrefix(strings.ToLower(parsed.Resource), "cluster:") || len(parsed.Resource) == len("cluster:") {
		return "", &native.Failure{
			Message:     "ResourceArn must be the ARN of a DB cluster, such as arn:aws:rds:us-east-1:123456789012:cluster:app.",
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return strings.ToLower(value), nil
}

// streamEquivalent treats a cluster ARN that differs only in case as the template's, because RDS stores it in
// lowercase, and a KMS key ARN as the key ID it names, either way round.
func streamEquivalent(path string, template any, aws any) bool {
	t, okT := template.(string)
	a, okA := aws.(string)
	if !okT || !okA {
		return false
	}
	switch path {
	case "ResourceArn":
		return strings.EqualFold(t, a)
	case "KmsKeyId":
		return t == a || strings.HasSuffix(a, ":key/"+t) || strings.HasSuffix(t, ":key/"+a)
	}
	return false
}

// sameAlias treats a KMS key AWS reports as the alias the template sets. An alias can't be told from another key
// without asking KMS, so UCP uses it only where the last deploy set the same alias (see
// native.Options.RecordedEquivalent), and KmsKeyId can't change after create.
func sameAlias(path string, template any, aws any) bool {
	t, okT := template.(string)
	_, okA := aws.(string)
	return path == "KmsKeyId" && okT && okA && (strings.HasPrefix(t, "alias/") || strings.Contains(t, ":alias/"))
}

// describe returns the cluster the ARN names, or ErrNotFound.
func (h *ActivityStream) describe(ctx context.Context, region string, identifier string) (*types.DBCluster, error) {
	out, err := h.api.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "DBClusterNotFoundFault")
	}
	if len(out.DBClusters) == 0 {
		return nil, native.ErrNotFound
	}
	return &out.DBClusters[0], nil
}

// streaming reports whether the cluster's stream is starting or started, the states in which AWS has it.
func streaming(c *types.DBCluster) bool {
	return c.ActivityStreamStatus == types.ActivityStreamStatusStarting || c.ActivityStreamStatus == types.ActivityStreamStatusStarted
}

func streamProperties(c *types.DBCluster) map[string]any {
	p := map[string]any{}
	setString(p, "ResourceArn", c.DBClusterArn)
	setString(p, "KmsKeyId", c.ActivityStreamKmsKeyId)
	setString(p, "KinesisStreamName", c.ActivityStreamKinesisStreamName)
	if c.ActivityStreamMode != "" {
		p["Mode"] = string(c.ActivityStreamMode)
	}
	return p
}

// Read returns the stream's properties. A cluster whose stream is stopped, or stopping, has none.
func (h *ActivityStream) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	cluster, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	if !streaming(cluster) {
		return nil, native.ErrNotFound
	}
	return streamProperties(cluster), nil
}

// Create starts the stream, applied at once.
func (h *ActivityStream) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	_, err = h.api.StartActivityStream(ctx, &rds.StartActivityStreamInput{
		ResourceArn:      aws.String(identifier),
		Mode:             types.ActivityStreamMode(awsapi.String(request.Properties, "Mode")),
		KmsKeyId:         aws.String(awsapi.String(request.Properties, "KmsKeyId")),
		ApplyImmediately: aws.Bool(true),
	}, inRegion(request.Region), retryThrottlesOnly)
	if err != nil {
		return "", awsapi.Rejected(err)
	}
	return identifier, nil
}

// Update changes nothing, because every property of the type is create-only.
func (h *ActivityStream) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete stops the stream. The type's onDelete is reset, so UCP calls Reset, which does the same.
func (h *ActivityStream) Delete(ctx context.Context, region string, identifier string) error {
	return h.Reset(ctx, region, identifier)
}

// Reset stops the stream, applied at once, which is AWS's default for a cluster. A stream AWS is already stopping, or
// a cluster AWS is deleting, which stops its stream, needs no call. When AWS rejects the stop for the cluster's state,
// Reset reads the cluster again, because a stop or a delete may have started since the first read.
func (h *ActivityStream) Reset(ctx context.Context, region string, identifier string) error {
	cluster, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	if done, err := stopped(cluster); done || err != nil {
		return err
	}
	_, err = h.api.StopActivityStream(ctx, &rds.StopActivityStreamInput{
		ResourceArn:      aws.String(identifier),
		ApplyImmediately: aws.Bool(true),
	}, inRegion(region))
	if awsapi.HasCode(err, "InvalidDBClusterStateFault") {
		cluster, describeErr := h.describe(ctx, region, identifier)
		if describeErr != nil {
			return describeErr
		}
		if done, stopErr := stopped(cluster); done || stopErr != nil {
			return stopErr
		}
	}
	return awsapi.Rejected(awsapi.NotFound(err, "DBClusterNotFoundFault"))
}

// stopped reports whether the cluster's stream needs no stop call: AWS is stopping it or deleting the cluster, or it
// is already stopped, which returns ErrNotFound.
func stopped(c *types.DBCluster) (bool, error) {
	switch {
	case c.ActivityStreamStatus == types.ActivityStreamStatusStopped || c.ActivityStreamStatus == "":
		return true, native.ErrNotFound
	case c.ActivityStreamStatus == types.ActivityStreamStatusStopping, strings.EqualFold(aws.ToString(c.Status), "deleting"):
		return true, nil
	}
	return false, nil
}

// List returns the stream of every cluster in the region that has one starting or started.
func (h *ActivityStream) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var marker *string
	for {
		page, err := h.api.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{Marker: marker}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.DBClusters {
			c := &page.DBClusters[i]
			if !streaming(c) {
				continue
			}
			out = append(out, native.Resource{Identifier: strings.ToLower(aws.ToString(c.DBClusterArn)), Properties: streamProperties(c)})
		}
		if marker = page.Marker; marker == nil {
			return out, nil
		}
	}
}

// Progress reads the stream's status. A create waits for started. A stream that is stopped or stopping reads as
// absent, as Read reports it, because AWS then didn't start it, or something stopped it since, so UCP fails the create
// and the next deploy starts the stream again. A delete waits until the stream is stopped, or the cluster is gone.
func (h *ActivityStream) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	cluster, err := h.describe(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	status := cluster.ActivityStreamStatus
	switch {
	case op == native.OperationDelete && (status == types.ActivityStreamStatusStopped || status == ""):
		return native.Progress{}, native.ErrNotFound
	case op == native.OperationDelete:
		return native.Progress{State: string(status), Waiting: string(types.ActivityStreamStatusStopped)}, nil
	case status == types.ActivityStreamStatusStarted:
		return native.Progress{Done: true, State: string(status)}, nil
	case status == types.ActivityStreamStatusStarting:
		return native.Progress{State: string(status), Waiting: string(types.ActivityStreamStatusStarted)}, nil
	}
	return native.Progress{}, native.ErrNotFound
}
