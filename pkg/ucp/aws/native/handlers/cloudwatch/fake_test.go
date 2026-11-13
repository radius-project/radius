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

package cloudwatch

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// fakeAPI is an in-memory CloudWatch API for anomaly detectors. It keeps detectors in an awsfake store, keyed by
// their identifier, in the type's property format, so the suite's Cloud Control sees the same resources. Like AWS,
// it returns every detector of one metric with SingleMetricAnomalyDetector set, whichever form created it, and with
// AccountId set to the current account for a detector created without one. A detector created with another account's
// AccountId is another detector, as a monitoring account sees one of a source account. It leaves out the deprecated
// top-level fields AWS also returns, which the handler never reads. It rejects the requests CloudWatch's API reference
// says AWS rejects (see api_test.go). It also serves the STS call for the current account.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	// account is the current account, and omitAccount makes a detector created without AccountId come back without
	// one.
	account     string
	omitAccount bool

	mu    sync.Mutex
	calls []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error

	// pageSize, when set, is how many detectors a DescribeAnomalyDetectors page holds.
	pageSize int
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, errs: map[string]error{}, account: fakeAccount}
}

// fakeAccount is the current account of the fake, and sourceAccount another account a monitoring account sees.
const (
	fakeAccount   = "123456789012"
	sourceAccount = "111122223333"
)

func (f *fakeAPI) GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	opts := sts.Options{}
	for _, fn := range optFns {
		fn(&opts)
	}
	f.calls = append(f.calls, "GetCallerIdentity "+opts.Region)
	if err := f.errs["GetCallerIdentity"]; err != nil {
		return nil, err
	}
	return &sts.GetCallerIdentityOutput{Account: aws.String(f.account)}, nil
}

// owned returns id with the current account when it has none, as AWS reads a detector's AccountId.
func (f *fakeAPI) owned(id identity) identity {
	if id.AccountId == "" && !id.isMath() {
		id.AccountId = f.account
	}
	return id
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// call records a call and the region its options set.
func (f *fakeAPI) call(name string, optFns []func(*cloudwatch.Options)) error {
	opts := cloudwatch.Options{}
	for _, fn := range optFns {
		fn(&opts)
	}
	f.calls = append(f.calls, name+" "+opts.Region)
	return f.errs[name]
}

// detector returns the detector stored under identifier as AWS returns it.
func (f *fakeAPI) detector(identifier string) (types.AnomalyDetector, error) {
	props := f.store.Resource(anomalyDetectorType, identifier)
	d := types.AnomalyDetector{}
	t, single, err := parseTemplate(props)
	if err != nil {
		return d, err
	}
	config, err := configuration(props)
	if err != nil {
		return d, err
	}
	d.Configuration = config
	d.MetricCharacteristics = t.MetricCharacteristics
	d.MetricMathAnomalyDetector = t.MetricMathAnomalyDetector
	if d.MetricMathAnomalyDetector == nil {
		d.SingleMetricAnomalyDetector = single
		if single != nil && aws.ToString(single.AccountId) == "" && !f.omitAccount {
			single.AccountId = aws.String(f.account)
		}
	}
	d.AnomalyDetectorId = aws.String(identifier)
	d.StateValue = types.AnomalyDetectorStateValuePendingTraining
	return d, nil
}

func (f *fakeAPI) PutAnomalyDetector(ctx context.Context, in *cloudwatch.PutAnomalyDetectorInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutAnomalyDetectorOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("PutAnomalyDetector", optFns); err != nil {
		return nil, err
	}
	single, err := checkDetector(in.SingleMetricAnomalyDetector, in.MetricMathAnomalyDetector, &types.SingleMetricAnomalyDetector{
		//nolint:staticcheck // AWS still takes the deprecated fields, and rejects them next to the detector forms.
		Namespace: in.Namespace, MetricName: in.MetricName, Dimensions: in.Dimensions, Stat: in.Stat,
	})
	if err != nil {
		return nil, err
	}
	if err := checkConfiguration(in.Configuration); err != nil {
		return nil, err
	}
	put := *in
	put.SingleMetricAnomalyDetector = single
	in = &put
	if math := in.MetricMathAnomalyDetector; math != nil {
		// Like AWS, the detector has ReturnData true on a query the put leaves it unset on.
		queries := slices.Clone(math.MetricDataQueries)
		for i := range queries {
			if queries[i].ReturnData == nil {
				queries[i].ReturnData = aws.Bool(true)
			}
		}
		in = &cloudwatch.PutAnomalyDetectorInput{
			MetricCharacteristics: in.MetricCharacteristics, Configuration: in.Configuration,
			MetricMathAnomalyDetector: &types.MetricMathAnomalyDetector{MetricDataQueries: queries},
		}
	}
	props, err := awsapi.EncodeObject(&cloudwatch.PutAnomalyDetectorInput{
		MetricCharacteristics: in.MetricCharacteristics, MetricMathAnomalyDetector: in.MetricMathAnomalyDetector,
		SingleMetricAnomalyDetector: in.SingleMetricAnomalyDetector,
	})
	if err != nil {
		return nil, err
	}
	// An empty configuration is AWS's default, which the detector then has instead of a configuration.
	if config := configurationProperties(in.Configuration); len(config) > 0 {
		if ranges, _ := config["ExcludedTimeRanges"].([]any); len(ranges) == 0 {
			delete(config, "ExcludedTimeRanges")
		}
		if len(config) > 0 {
			props["Configuration"] = config
		}
	}
	identifier, err := NewAnomalyDetector(f, f).Identifier(props)
	if err != nil {
		return nil, err
	}
	if identifier == "" {
		return nil, fault("MissingParameter")
	}
	// A detector of the current account is one detector, whether the put names the account or not, so it is kept
	// under the identifier without the account.
	if id, err := parseIdentifier(identifier); err == nil && id.AccountId == f.account {
		id.AccountId = ""
		if identifier, err = id.encode(); err != nil {
			return nil, err
		}
	}
	if err := f.failer.Before(); err != nil {
		return nil, err
	}
	if existing := f.store.Resource(anomalyDetectorType, identifier); existing != nil {
		// A put of a detector AWS has keeps the form that created it, and replaces its settings. Like AWS, a put
		// without a Configuration clears the detector's configuration.
		for _, k := range []string{"Configuration", "MetricCharacteristics"} {
			delete(existing, k)
			if v, ok := props[k]; ok {
				existing[k] = v
			}
		}
		props = existing
	}
	props["Id"] = identifier
	f.store.Put(anomalyDetectorType, identifier, props)
	return &cloudwatch.PutAnomalyDetectorOutput{}, f.failer.After()
}

func (f *fakeAPI) DescribeAnomalyDetectors(ctx context.Context, in *cloudwatch.DescribeAnomalyDetectorsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAnomalyDetectorsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DescribeAnomalyDetectors", optFns); err != nil {
		return nil, err
	}
	if err := checkDescribe(in); err != nil {
		return nil, err
	}
	kinds := in.AnomalyDetectorTypes
	if len(kinds) == 0 {
		kinds = []types.AnomalyDetectorType{types.AnomalyDetectorTypeSingleMetric}
	}
	matches := []types.AnomalyDetector{}
	for _, identifier := range f.store.List(anomalyDetectorType) {
		d, err := f.detector(identifier)
		if err != nil {
			return nil, err
		}
		kind := types.AnomalyDetectorTypeSingleMetric
		if d.MetricMathAnomalyDetector != nil {
			kind = types.AnomalyDetectorTypeMetricMath
		}
		if !slices.Contains(kinds, kind) || !matchesFilter(d, in) {
			continue
		}
		matches = append(matches, d)
	}
	start := 0
	if in.NextToken != nil {
		var err error
		if start, err = strconv.Atoi(aws.ToString(in.NextToken)); err != nil || start < 0 || start > len(matches) {
			return nil, fault("InvalidNextToken")
		}
	}
	size := f.pageSize
	if limit := int(aws.ToInt32(in.MaxResults)); limit > 0 && (size == 0 || limit < size) {
		size = limit
	}
	out := &cloudwatch.DescribeAnomalyDetectorsOutput{AnomalyDetectors: matches[start:]}
	if size > 0 && len(matches)-start > size {
		out.AnomalyDetectors = matches[start : start+size]
		out.NextToken = aws.String(strconv.Itoa(start + size))
	}
	return out, nil
}

// matchesFilter reports whether a detector of one metric has the namespace, metric name, and dimensions the input
// filters by, as AWS filters.
func matchesFilter(d types.AnomalyDetector, in *cloudwatch.DescribeAnomalyDetectorsInput) bool {
	s := d.SingleMetricAnomalyDetector
	if s == nil {
		return in.Namespace == nil && in.MetricName == nil && len(in.Dimensions) == 0
	}
	if in.Namespace != nil && aws.ToString(in.Namespace) != aws.ToString(s.Namespace) {
		return false
	}
	if in.MetricName != nil && aws.ToString(in.MetricName) != aws.ToString(s.MetricName) {
		return false
	}
	have := map[string]string{}
	for _, dim := range s.Dimensions {
		have[aws.ToString(dim.Name)] = aws.ToString(dim.Value)
	}
	for _, dim := range in.Dimensions {
		if v, ok := have[aws.ToString(dim.Name)]; !ok || v != aws.ToString(dim.Value) {
			return false
		}
	}
	return true
}

func (f *fakeAPI) DeleteAnomalyDetector(ctx context.Context, in *cloudwatch.DeleteAnomalyDetectorInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.DeleteAnomalyDetectorOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteAnomalyDetector", optFns); err != nil {
		return nil, err
	}
	single, err := checkDetector(in.SingleMetricAnomalyDetector, in.MetricMathAnomalyDetector, &types.SingleMetricAnomalyDetector{
		//nolint:staticcheck // AWS still takes the deprecated fields, and rejects them next to the detector forms.
		Namespace: in.Namespace, MetricName: in.MetricName, Dimensions: in.Dimensions, Stat: in.Stat,
	})
	if err != nil {
		return nil, err
	}
	props, err := awsapi.EncodeObject(&cloudwatch.PutAnomalyDetectorInput{
		MetricMathAnomalyDetector: in.MetricMathAnomalyDetector, SingleMetricAnomalyDetector: single,
	})
	if err != nil {
		return nil, err
	}
	id, err := identityOf(props)
	if err != nil {
		return nil, err
	}
	for _, identifier := range f.store.List(anomalyDetectorType) {
		d, err := f.detector(identifier)
		if err != nil {
			return nil, err
		}
		stored, err := detectorIdentity(&d)
		if err != nil {
			return nil, err
		}
		a, errA := f.owned(id).encode()
		b, errB := f.owned(stored).encode()
		if errA == nil && errB == nil && a == b {
			if err := f.failer.Before(); err != nil {
				return nil, err
			}
			f.store.Remove(anomalyDetectorType, identifier)
			return &cloudwatch.DeleteAnomalyDetectorOutput{}, f.failer.After()
		}
	}
	return nil, fault("ResourceNotFoundException")
}

// The limits CloudWatch's API reference gives for anomaly detectors.
const (
	maxDimensions     = 30
	maxExcludedRanges = 10
	maxDetectorIDs    = 50
	maxResults        = 100
)

// statPattern is the pattern CloudWatch's API reference gives for the statistic of a detector of one metric.
var statPattern = regexp.MustCompile(`^(?:(SampleCount|Average|Sum|Minimum|Maximum|IQM|(p|tc|tm|ts|wm)(\d{1,2}(\.\d{0,10})?|100)|[ou]\d+(\.\d*)?)(_E|_L|_H)?|(TM|TC|TS|WM)\(((((\d{1,2})(\.\d{0,10})?|100(\.0{0,10})?)%)?:((\d{1,2})(\.\d{0,10})?|100(\.0{0,10})?)%|((\d{1,2})(\.\d{0,10})?|100(\.0{0,10})?)%:(((\d{1,2})(\.\d{0,10})?|100(\.0{0,10})?)%)?)\)|(TM|TC|TS|WM|PR)\(((\d+(\.\d{0,10})?|(\d+(\.\d{0,10})?[Ee][+-]?\d+)):((\d+(\.\d{0,10})?|(\d+(\.\d{0,10})?[Ee][+-]?\d+)))?|((\d+(\.\d{0,10})?|(\d+(\.\d{0,10})?[Ee][+-]?\d+)))?:(\d+(\.\d{0,10})?|(\d+(\.\d{0,10})?[Ee][+-]?\d+)))\))$`)

// queryIDPattern is the form of a metric data query's Id: letters, numbers, and underscores, starting with a lowercase
// letter.
var queryIDPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9_]*$`)

// checkLength returns ValidationError when s is set and its length, in characters, is outside min to max.
func checkLength(s *string, min int, max int) error {
	if s != nil && (utf8.RuneCountInString(*s) < min || utf8.RuneCountInString(*s) > max) {
		return fault("ValidationError")
	}
	return nil
}

// checkDetector checks the forms of a detector a put or delete names: a detector of one metric, a metric math
// detector, or the deprecated top-level fields, which can't be combined. It returns the detector of one metric the
// call names, in either form, or nil.
func checkDetector(single *types.SingleMetricAnomalyDetector, math *types.MetricMathAnomalyDetector, topLevel *types.SingleMetricAnomalyDetector) (*types.SingleMetricAnomalyDetector, error) {
	hasTopLevel := topLevel.Namespace != nil || topLevel.MetricName != nil || topLevel.Stat != nil || len(topLevel.Dimensions) > 0
	if btoi(single != nil)+btoi(math != nil)+btoi(hasTopLevel) > 1 {
		return nil, fault("InvalidParameterCombination")
	}
	if hasTopLevel {
		single = topLevel
	}
	if single != nil {
		return single, checkSingle(single)
	}
	if math != nil {
		return nil, checkMath(math)
	}
	return nil, nil
}

// checkSingle checks the fields of a detector of one metric.
func checkSingle(s *types.SingleMetricAnomalyDetector) error {
	if err := checkLength(s.AccountId, 1, 255); err != nil {
		return err
	}
	if err := checkMetric(s.Namespace, s.MetricName, s.Dimensions); err != nil {
		return err
	}
	if s.Stat != nil && (utf8.RuneCountInString(*s.Stat) > 50 || !statPattern.MatchString(*s.Stat)) {
		return fault("ValidationError")
	}
	return nil
}

// checkMetric checks a metric's namespace, name, and dimensions, any of which may be unset.
func checkMetric(namespace *string, name *string, dimensions []types.Dimension) error {
	if err := checkLength(namespace, 1, 255); err != nil {
		return err
	}
	if namespace != nil && strings.HasPrefix(*namespace, ":") {
		return fault("ValidationError")
	}
	if err := checkLength(name, 1, 255); err != nil {
		return err
	}
	if len(dimensions) > maxDimensions {
		return fault("ValidationError")
	}
	for _, d := range dimensions {
		if d.Name == nil || d.Value == nil {
			return fault("MissingParameter")
		}
		if err := checkLength(d.Name, 1, 255); err != nil {
			return err
		}
		if err := checkLength(d.Value, 1, 1024); err != nil {
			return err
		}
	}
	return nil
}

// checkMath checks the queries of a metric math detector. A query without ReturnData counts as one with ReturnData
// true, as AWS returns it.
func checkMath(m *types.MetricMathAnomalyDetector) error {
	returned := 0
	for _, q := range m.MetricDataQueries {
		if q.Id == nil {
			return fault("MissingParameter")
		}
		if err := checkLength(q.Id, 1, 255); err != nil {
			return err
		}
		if !queryIDPattern.MatchString(*q.Id) {
			return fault("InvalidParameterValue")
		}
		if q.Expression != nil && q.MetricStat != nil {
			return fault("InvalidParameterCombination")
		}
		if q.Expression == nil && q.MetricStat == nil {
			return fault("MissingParameter")
		}
		if err := checkLength(q.Expression, 1, 2048); err != nil {
			return err
		}
		if err := checkLength(q.AccountId, 1, 255); err != nil {
			return err
		}
		if q.Period != nil && *q.Period < 1 {
			return fault("ValidationError")
		}
		if s := q.MetricStat; s != nil {
			if s.Metric == nil || s.Period == nil || s.Stat == nil {
				return fault("MissingParameter")
			}
			if *s.Period < 1 {
				return fault("ValidationError")
			}
			if err := checkMetric(s.Metric.Namespace, s.Metric.MetricName, s.Metric.Dimensions); err != nil {
				return err
			}
		}
		if aws.ToBool(q.ReturnData) || q.ReturnData == nil {
			returned++
		}
	}
	if returned != 1 {
		return fault("InvalidParameterValue")
	}
	return nil
}

// checkConfiguration checks a detector's configuration, which may be nil.
func checkConfiguration(c *types.AnomalyDetectorConfiguration) error {
	if c == nil {
		return nil
	}
	if len(c.ExcludedTimeRanges) > maxExcludedRanges {
		return fault("InvalidParameterValue")
	}
	for _, r := range c.ExcludedTimeRanges {
		if r.StartTime == nil || r.EndTime == nil {
			return fault("MissingParameter")
		}
	}
	if zone := c.MetricTimezone; zone != nil {
		if err := checkLength(zone, 0, 50); err != nil {
			return err
		}
		if _, err := time.LoadLocation(*zone); err != nil || *zone == "Local" {
			return fault("InvalidParameterValue")
		}
	}
	if c.AdaptationSpeed != "" && !slices.Contains(c.AdaptationSpeed.Values(), c.AdaptationSpeed) {
		return fault("ValidationError")
	}
	if c.OutlierFilteringStrength != "" && !slices.Contains(c.OutlierFilteringStrength.Values(), c.OutlierFilteringStrength) {
		return fault("ValidationError")
	}
	return nil
}

// checkDescribe checks the filters of a DescribeAnomalyDetectors request.
func checkDescribe(in *cloudwatch.DescribeAnomalyDetectorsInput) error {
	if len(in.AnomalyDetectorIds) > 0 && (in.Namespace != nil || in.MetricName != nil || len(in.Dimensions) > 0 || len(in.AnomalyDetectorTypes) > 0) {
		return fault("InvalidParameterCombination")
	}
	if len(in.AnomalyDetectorIds) > maxDetectorIDs || len(in.AnomalyDetectorTypes) > 2 {
		return fault("ValidationError")
	}
	for _, kind := range in.AnomalyDetectorTypes {
		if !slices.Contains(kind.Values(), kind) {
			return fault("ValidationError")
		}
	}
	if err := checkMetric(in.Namespace, in.MetricName, in.Dimensions); err != nil {
		return err
	}
	if in.MaxResults != nil && *in.MaxResults < 1 {
		return fault("ValidationError")
	}
	if in.MaxResults != nil && *in.MaxResults > maxResults {
		return fault("InvalidParameterValue")
	}
	return nil
}
