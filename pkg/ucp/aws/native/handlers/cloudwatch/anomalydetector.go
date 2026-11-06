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

// Package cloudwatch has the native handler for CloudWatch anomaly detectors, which Cloud Control can't create in
// any region.
package cloudwatch

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const anomalyDetectorType = "AWS::CloudWatch::AnomalyDetector"

// API is the part of the CloudWatch client the handler uses.
type API interface {
	PutAnomalyDetector(ctx context.Context, params *cloudwatch.PutAnomalyDetectorInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutAnomalyDetectorOutput, error)
	DescribeAnomalyDetectors(ctx context.Context, params *cloudwatch.DescribeAnomalyDetectorsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAnomalyDetectorsOutput, error)
	DeleteAnomalyDetector(ctx context.Context, params *cloudwatch.DeleteAnomalyDetectorInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DeleteAnomalyDetectorOutput, error)
}

// AccountAPI is the part of the STS client the handler uses, to learn the current account.
type AccountAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

func inRegion(region string) func(*cloudwatch.Options) {
	return func(o *cloudwatch.Options) { o.Region = region }
}

var (
	_ native.Handler            = (*AnomalyDetector)(nil)
	_ native.IdentifierEncoder  = (*AnomalyDetector)(nil)
	_ native.IdentifierResolver = (*AnomalyDetector)(nil)
)

// notFoundCodes are the codes CloudWatch returns for an anomaly detector it doesn't have.
var notFoundCodes = []string{"ResourceNotFoundException", "ResourceNotFound"}

// AnomalyDetector serves AWS::CloudWatch::AnomalyDetector. A detector has no name: the metric and statistic it
// watches, or its metric math queries, identify it. Its identifier, Id, is that identity as JSON, the same for the
// SingleMetricAnomalyDetector form and the older top-level Namespace, MetricName, Dimensions, and Stat form.
// PutAnomalyDetector creates or updates a detector, and takes effect at once.
//
// A detector of one metric without AccountId is in the current account, as the CloudWatch API defines it. AWS may
// return that detector with the current account's ID or without one, and a monitoring account also sees detectors of
// the same metric in its source accounts. So the handler compares a detector's account only after it fills in the
// current account, which it asks STS for when a comparison needs it. An identifier has no AccountId for a detector of
// the current account, so a template that names the current account and one that leaves it out give one identifier
// (see ResolveIdentifier), and two aliases can't both hold the detector.
type AnomalyDetector struct {
	api      API
	accounts AccountAPI
}

// NewAnomalyDetector returns the handler for CloudWatch anomaly detectors. It learns the current account with accounts.
func NewAnomalyDetector(api API, accounts AccountAPI) *AnomalyDetector {
	return &AnomalyDetector{api: api, accounts: accounts}
}

// currentAccount returns a function that returns the account of the handler's credentials. It asks STS on its first
// call only, so an operation that never needs the account makes no STS call.
func (h *AnomalyDetector) currentAccount(ctx context.Context, region string) func() (string, error) {
	account := ""
	return func() (string, error) {
		if account != "" {
			return account, nil
		}
		out, err := h.accounts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, func(o *sts.Options) { o.Region = region })
		if err != nil {
			return "", err
		}
		if account = aws.ToString(out.Account); account == "" {
			return "", errors.New("STS returned no account ID for the AWS credentials")
		}
		return account, nil
	}
}

// TypeName returns AWS::CloudWatch::AnomalyDetector.
func (h *AnomalyDetector) TypeName() string {
	return anomalyDetectorType
}

// Options returns how UCP uses the handler.
func (h *AnomalyDetector) Options() native.Options {
	return native.Options{
		List:       native.ListRegion,
		Recovery:   native.RecoverByIdentifier,
		Equivalent: equivalent,
	}
}

// identity is what identifies a detector: one metric and its statistic, or metric math queries.
type identity struct {
	AccountId         string      `json:"AccountId,omitempty"`
	Namespace         string      `json:"Namespace,omitempty"`
	MetricName        string      `json:"MetricName,omitempty"`
	Dimensions        []dimension `json:"Dimensions,omitempty"`
	Stat              string      `json:"Stat,omitempty"`
	MetricDataQueries []any       `json:"MetricDataQueries,omitempty"`
}

type dimension struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

func (id identity) isMath() bool {
	return len(id.MetricDataQueries) > 0
}

// encode returns the identity as JSON, the identifier.
func (id identity) encode() (string, error) {
	b, err := json.Marshal(id)
	return string(b), err
}

// same reports whether id and other identify the same detector. An identity without AccountId is in the current
// account, which account returns. It calls account only when the rest of the identities match and only one of them
// has an AccountId.
func (id identity) same(other identity, account func() (string, error)) (bool, error) {
	a, b := id, other
	a.AccountId, b.AccountId = "", ""
	if !a.equal(b) {
		return false, nil
	}
	if (id.AccountId == "") == (other.AccountId == "") {
		return id.AccountId == other.AccountId, nil
	}
	current, err := account()
	if err != nil {
		return false, err
	}
	return cmp.Or(id.AccountId, current) == cmp.Or(other.AccountId, current), nil
}

// equal reports whether id and other are the same identity, account included.
func (id identity) equal(other identity) bool {
	a, errA := id.encode()
	b, errB := other.encode()
	return errA == nil && errB == nil && a == b
}

// dimensions returns dims sorted by name and value, because a detector's dimensions are a set.
func dimensions(dims []types.Dimension) []dimension {
	out := []dimension{}
	for _, d := range dims {
		out = append(out, dimension{Name: aws.ToString(d.Name), Value: aws.ToString(d.Value)})
	}
	slices.SortFunc(out, func(a, b dimension) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Value, b.Value))
	})
	return out
}

func sdkDimensions(dims []dimension) []types.Dimension {
	out := []types.Dimension{}
	for _, d := range dims {
		out = append(out, types.Dimension{Name: aws.String(d.Name), Value: aws.String(d.Value)})
	}
	return out
}

// mathIdentity returns the identity of metric math queries, with the dimensions of each metric sorted.
func mathIdentity(math *types.MetricMathAnomalyDetector) (identity, error) {
	queries := slices.Clone(math.MetricDataQueries)
	for i, q := range queries {
		if q.MetricStat != nil && q.MetricStat.Metric != nil {
			stat, metric := *q.MetricStat, *q.MetricStat.Metric
			metric.Dimensions = sdkDimensions(dimensions(metric.Dimensions))
			stat.Metric = &metric
			queries[i].MetricStat = &stat
		}
	}
	encoded, err := awsapi.Encode(queries)
	if err != nil {
		return identity{}, err
	}
	list, _ := encoded.([]any)
	return identity{MetricDataQueries: list}, nil
}

// singleIdentity returns the identity of a detector of one metric.
func singleIdentity(single *types.SingleMetricAnomalyDetector) identity {
	if single == nil {
		return identity{}
	}
	return identity{
		AccountId:  aws.ToString(single.AccountId),
		Namespace:  aws.ToString(single.Namespace),
		MetricName: aws.ToString(single.MetricName),
		Dimensions: dimensions(single.Dimensions),
		Stat:       aws.ToString(single.Stat),
	}
}

// template is a detector's properties. The schema keeps the API's older form of a detector of one metric: Namespace,
// MetricName, Dimensions, and Stat at the top level, which the handler sends as SingleMetricAnomalyDetector.
type template struct {
	SingleMetricAnomalyDetector *types.SingleMetricAnomalyDetector
	MetricMathAnomalyDetector   *types.MetricMathAnomalyDetector
	MetricCharacteristics       *types.MetricCharacteristics

	Namespace  *string
	MetricName *string
	Dimensions []types.Dimension
	Stat       *string
}

// parseTemplate reads the properties. single is the detector of one metric they give, in either form, or nil.
func parseTemplate(properties map[string]any) (t *template, single *types.SingleMetricAnomalyDetector, err error) {
	t = &template{}
	if err := awsapi.Decode(awsapi.Without(properties, "Id", "Configuration"), t); err != nil {
		return nil, nil, err
	}
	single = t.SingleMetricAnomalyDetector
	if single == nil && t.hasTopLevel() {
		single = &types.SingleMetricAnomalyDetector{Namespace: t.Namespace, MetricName: t.MetricName, Dimensions: t.Dimensions, Stat: t.Stat}
	}
	return t, single, nil
}

func (t *template) hasTopLevel() bool {
	return t.Namespace != nil || t.MetricName != nil || t.Stat != nil || len(t.Dimensions) > 0
}

// identityOf returns the identity the properties give.
func identityOf(properties map[string]any) (identity, error) {
	t, single, err := parseTemplate(properties)
	if err != nil {
		return identity{}, err
	}
	if t.MetricMathAnomalyDetector != nil {
		return mathIdentity(t.MetricMathAnomalyDetector)
	}
	return singleIdentity(single), nil
}

// detectorIdentity returns the identity of a detector AWS returned.
func detectorIdentity(d *types.AnomalyDetector) (identity, error) {
	if d.MetricMathAnomalyDetector != nil {
		return mathIdentity(d.MetricMathAnomalyDetector)
	}
	return singleIdentity(d.SingleMetricAnomalyDetector), nil
}

// parseIdentifier returns the identity of an identifier.
func parseIdentifier(identifier string) (identity, error) {
	id := identity{}
	if err := json.Unmarshal([]byte(identifier), &id); err != nil || (id.Namespace == "" && !id.isMath()) {
		return id, &native.Failure{
			Message:     fmt.Sprintf("%q is not the identifier of a CloudWatch anomaly detector: it must be the JSON object the Id property returns", identifier),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return id, nil
}

// Identifier returns the detector's identity as JSON, or "" when the properties don't name a metric or metric math
// queries.
func (h *AnomalyDetector) Identifier(properties map[string]any) (string, error) {
	id, err := identityOf(properties)
	if err != nil {
		return "", err
	}
	if id.Namespace == "" && !id.isMath() {
		return "", nil
	}
	return id.encode()
}

// find returns the detector with the identity, or ErrNotFound.
func (h *AnomalyDetector) find(ctx context.Context, region string, id identity) (*types.AnomalyDetector, error) {
	input := &cloudwatch.DescribeAnomalyDetectorsInput{AnomalyDetectorTypes: []types.AnomalyDetectorType{types.AnomalyDetectorTypeMetricMath}}
	if !id.isMath() {
		input = &cloudwatch.DescribeAnomalyDetectorsInput{
			AnomalyDetectorTypes: []types.AnomalyDetectorType{types.AnomalyDetectorTypeSingleMetric},
			Namespace:            aws.String(id.Namespace),
			MetricName:           aws.String(id.MetricName),
			Dimensions:           sdkDimensions(id.Dimensions),
		}
	}
	var found *types.AnomalyDetector
	account := h.currentAccount(ctx, region)
	err := h.describe(ctx, region, input, func(d *types.AnomalyDetector, other identity) error {
		if found != nil {
			return nil
		}
		same, err := id.same(other, account)
		if same {
			found = d
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, native.ErrNotFound
	}
	return found, nil
}

// describe calls each with every detector DescribeAnomalyDetectors returns for input, and its identity, and stops at
// the first error.
func (h *AnomalyDetector) describe(ctx context.Context, region string, input *cloudwatch.DescribeAnomalyDetectorsInput, each func(d *types.AnomalyDetector, id identity) error) error {
	for {
		page, err := h.api.DescribeAnomalyDetectors(ctx, input, inRegion(region))
		if err != nil {
			return err
		}
		for i := range page.AnomalyDetectors {
			d := &page.AnomalyDetectors[i]
			id, err := detectorIdentity(d)
			if err != nil {
				return err
			}
			if err := each(d, id); err != nil {
				return err
			}
		}
		if page.NextToken == nil {
			return nil
		}
		input.NextToken = page.NextToken
	}
}

// Read returns the detector's properties.
func (h *AnomalyDetector) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	id, err := parseIdentifier(identifier)
	if err != nil {
		return nil, err
	}
	d, err := h.find(ctx, region, id)
	if err != nil {
		return nil, err
	}
	return properties(identifier, d, id)
}

// properties returns the properties of the detector d, whose identity is id. A detector of one metric is returned in
// both forms, SingleMetricAnomalyDetector and the top-level properties, so it matches a template that uses either.
func properties(identifier string, d *types.AnomalyDetector, id identity) (map[string]any, error) {
	p := map[string]any{
		"Id":                    identifier,
		"Configuration":         configurationProperties(d.Configuration),
		"MetricCharacteristics": d.MetricCharacteristics,
	}
	if id.isMath() {
		p["MetricMathAnomalyDetector"] = d.MetricMathAnomalyDetector
	} else {
		var dims []types.Dimension
		if d.SingleMetricAnomalyDetector != nil {
			dims = d.SingleMetricAnomalyDetector.Dimensions
		}
		p["Namespace"], p["MetricName"], p["Stat"], p["Dimensions"] = id.Namespace, id.MetricName, id.Stat, dims
		p["SingleMetricAnomalyDetector"] = &types.SingleMetricAnomalyDetector{
			AccountId:  aws.String(id.AccountId),
			Namespace:  aws.String(id.Namespace),
			MetricName: aws.String(id.MetricName),
			Stat:       aws.String(id.Stat),
			Dimensions: dims,
		}
	}
	return awsapi.EncodeObject(p)
}

// timeFormat is the format of the times of excluded ranges in the schema, such as 2019-07-01T23:59:59.
const timeFormat = "2006-01-02T15:04:05"

// configurationProperties returns a configuration in the schema's format: the API's MetricTimezone is MetricTimeZone,
// and times are in timeFormat, in UTC.
func configurationProperties(c *types.AnomalyDetectorConfiguration) map[string]any {
	if c == nil {
		return nil
	}
	ranges := []any{}
	for _, r := range c.ExcludedTimeRanges {
		ranges = append(ranges, map[string]any{
			"StartTime": aws.ToTime(r.StartTime).UTC().Format(timeFormat),
			"EndTime":   aws.ToTime(r.EndTime).UTC().Format(timeFormat),
		})
	}
	out := map[string]any{"ExcludedTimeRanges": ranges}
	if c.MetricTimezone != nil {
		out["MetricTimeZone"] = aws.ToString(c.MetricTimezone)
	}
	return out
}

// parseTime reads a time of an excluded range, in timeFormat, which is UTC, or RFC 3339.
func parseTime(value any) (time.Time, bool) {
	s, ok := value.(string)
	if !ok {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	t, err := time.Parse(timeFormat, s)
	return t, err == nil
}

// configuration returns the Configuration property as the API's configuration, or nil when it is unset.
func configuration(properties map[string]any) (*types.AnomalyDetectorConfiguration, error) {
	c, ok := properties["Configuration"].(map[string]any)
	if !ok {
		return nil, nil
	}
	out := &types.AnomalyDetectorConfiguration{}
	if zone := awsapi.String(c, "MetricTimeZone"); zone != "" {
		out.MetricTimezone = aws.String(zone)
	}
	ranges, _ := c["ExcludedTimeRanges"].([]any)
	for _, item := range ranges {
		r, _ := item.(map[string]any)
		start, okStart := parseTime(r["StartTime"])
		end, okEnd := parseTime(r["EndTime"])
		if !okStart || !okEnd {
			return nil, &native.Failure{
				Message:     "Configuration/ExcludedTimeRanges needs a StartTime and an EndTime in the format yyyy-MM-ddTHH:mm:ss, such as 2019-07-01T23:59:59",
				ClientError: true,
				Outcome:     native.OutcomeRejected,
			}
		}
		out.ExcludedTimeRanges = append(out.ExcludedTimeRanges, types.Range{StartTime: aws.Time(start), EndTime: aws.Time(end)})
	}
	return out, nil
}

// equivalent treats excluded time ranges as the same when their times are the same instants, such as
// 2019-07-01T23:59:59 and 2019-07-01T23:59:59Z.
func equivalent(path string, template any, aws any) bool {
	if path != "Configuration/ExcludedTimeRanges" {
		return false
	}
	t, okT := template.([]any)
	a, okA := aws.([]any)
	if !okT || !okA || len(t) != len(a) {
		return false
	}
	for i := range t {
		tr, _ := t[i].(map[string]any)
		ar, _ := a[i].(map[string]any)
		for _, key := range []string{"StartTime", "EndTime"} {
			tt, okT := parseTime(tr[key])
			at, okA := parseTime(ar[key])
			if !okT || !okA || !tt.Equal(at) {
				return false
			}
		}
	}
	return true
}

// put creates or updates the detector the properties give. A detector of one metric is always sent as
// SingleMetricAnomalyDetector, because the API's top-level form is deprecated.
func (h *AnomalyDetector) put(ctx context.Context, region string, properties map[string]any) error {
	t, single, err := parseTemplate(properties)
	if err != nil {
		return err
	}
	if forms := btoi(t.SingleMetricAnomalyDetector != nil) + btoi(t.hasTopLevel()) + btoi(t.MetricMathAnomalyDetector != nil); forms > 1 {
		return &native.Failure{
			Message:     "Set one of SingleMetricAnomalyDetector, MetricMathAnomalyDetector, or Namespace, MetricName, Dimensions, and Stat",
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	config, err := configuration(properties)
	if err != nil {
		return err
	}
	input := &cloudwatch.PutAnomalyDetectorInput{
		SingleMetricAnomalyDetector: single,
		MetricMathAnomalyDetector:   t.MetricMathAnomalyDetector,
		MetricCharacteristics:       t.MetricCharacteristics,
		Configuration:               config,
	}
	_, err = h.api.PutAnomalyDetector(ctx, input, inRegion(region))
	return err
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ResolveIdentifier returns identifier without AccountId when that is the current account, the form List and Create
// return. It asks STS for the current account only when the identifier has an AccountId. A detector of another
// account keeps its AccountId.
func (h *AnomalyDetector) ResolveIdentifier(ctx context.Context, region string, identifier string) (string, error) {
	id, err := parseIdentifier(identifier)
	if err != nil {
		return "", err
	}
	if id.AccountId == "" {
		return identifier, nil
	}
	current, err := h.currentAccount(ctx, region)()
	if err != nil {
		return "", err
	}
	if id.AccountId != current {
		return identifier, nil
	}
	id.AccountId = ""
	return id.encode()
}

// Create creates the detector with PutAnomalyDetector, and returns its identifier in the form ResolveIdentifier gives.
// Sending it again makes the same detector.
func (h *AnomalyDetector) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	identifier, err := h.Identifier(request.Properties)
	if err != nil {
		return "", err
	}
	if identifier, err = h.ResolveIdentifier(ctx, request.Region, identifier); err != nil {
		return "", err
	}
	if err := h.put(ctx, request.Region, request.Properties); err != nil {
		return "", err
	}
	return identifier, nil
}

// Update sends the template's properties with PutAnomalyDetector, which replaces the detector's configuration. Every
// other property is create-only. A configuration the template no longer sets goes back to AWS's default.
func (h *AnomalyDetector) Update(ctx context.Context, request native.UpdateRequest) error {
	return h.put(ctx, request.Region, request.Desired)
}

// Delete deletes the detector. One without AccountId is the current account's, which AWS deletes when the request has
// no AccountId.
func (h *AnomalyDetector) Delete(ctx context.Context, region string, identifier string) error {
	id, err := parseIdentifier(identifier)
	if err != nil {
		return err
	}
	input := &cloudwatch.DeleteAnomalyDetectorInput{}
	if id.isMath() {
		math := &types.MetricMathAnomalyDetector{}
		if err := awsapi.Decode(map[string]any{"MetricDataQueries": id.MetricDataQueries}, math); err != nil {
			return err
		}
		input.MetricMathAnomalyDetector = math
	} else {
		input.SingleMetricAnomalyDetector = &types.SingleMetricAnomalyDetector{
			Namespace:  aws.String(id.Namespace),
			MetricName: aws.String(id.MetricName),
			Dimensions: sdkDimensions(id.Dimensions),
			Stat:       aws.String(id.Stat),
		}
		if id.AccountId != "" {
			input.SingleMetricAnomalyDetector.AccountId = aws.String(id.AccountId)
		}
	}
	_, err = h.api.DeleteAnomalyDetector(ctx, input, inRegion(region))
	return awsapi.NotFound(err, notFoundCodes...)
}

// List returns every anomaly detector in the region. A detector of the current account gets the identifier a template
// without AccountId gives, so the identifier check finds an alias that created it.
func (h *AnomalyDetector) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	input := &cloudwatch.DescribeAnomalyDetectorsInput{
		AnomalyDetectorTypes: []types.AnomalyDetectorType{types.AnomalyDetectorTypeSingleMetric, types.AnomalyDetectorTypeMetricMath},
	}
	account := h.currentAccount(ctx, region)
	err := h.describe(ctx, region, input, func(d *types.AnomalyDetector, id identity) error {
		if id.AccountId != "" {
			current, err := account()
			if err != nil {
				return err
			}
			if id.AccountId == current {
				id.AccountId = ""
			}
		}
		identifier, err := id.encode()
		if err != nil {
			return err
		}
		p, err := properties(identifier, d, id)
		if err != nil {
			return err
		}
		out = append(out, native.Resource{Identifier: identifier, Properties: p})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Progress reads the detector. A create or update takes effect at once, and a delete waits for the detector to be
// gone.
func (h *AnomalyDetector) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	id, err := parseIdentifier(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if _, err := h.find(ctx, region, id); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{State: "present", Waiting: "deleted"}, nil
	}
	return native.Progress{Done: true}, nil
}
