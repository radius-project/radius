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
	"slices"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// fakeAPI is an in-memory CloudWatch API for anomaly detectors. It keeps detectors in an awsfake store, keyed by
// their identifier, in the type's property format, so the suite's Cloud Control sees the same resources. Like AWS,
// it returns every detector of one metric with SingleMetricAnomalyDetector set, whichever form created it. It leaves
// out the deprecated top-level fields AWS also returns, which the handler never reads.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu    sync.Mutex
	calls []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error

	// pageSize, when set, is how many detectors a DescribeAnomalyDetectors page holds.
	pageSize int
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, errs: map[string]error{}}
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
	if in.MetricMathAnomalyDetector != nil && in.SingleMetricAnomalyDetector != nil {
		return nil, fault("InvalidParameterCombinationException")
	}
	props, err := awsapi.EncodeObject(&cloudwatch.PutAnomalyDetectorInput{
		MetricCharacteristics: in.MetricCharacteristics, MetricMathAnomalyDetector: in.MetricMathAnomalyDetector,
		SingleMetricAnomalyDetector: in.SingleMetricAnomalyDetector,
	})
	if err != nil {
		return nil, err
	}
	if config := configurationProperties(in.Configuration); len(config) > 0 {
		props["Configuration"] = config
	}
	identifier, err := NewAnomalyDetector(f).Identifier(props)
	if err != nil {
		return nil, err
	}
	if identifier == "" {
		return nil, fault("MissingRequiredParameterException")
	}
	if err := f.failer.Before(); err != nil {
		return nil, err
	}
	if existing := f.store.Resource(anomalyDetectorType, identifier); existing != nil {
		// A put of a detector AWS has keeps the form that created it, and replaces its settings.
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
		start, _ = strconv.Atoi(aws.ToString(in.NextToken))
	}
	out := &cloudwatch.DescribeAnomalyDetectorsOutput{AnomalyDetectors: matches[start:]}
	if f.pageSize > 0 && len(matches)-start > f.pageSize {
		out.AnomalyDetectors = matches[start : start+f.pageSize]
		out.NextToken = aws.String(strconv.Itoa(start + f.pageSize))
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
	props, err := awsapi.EncodeObject(&cloudwatch.PutAnomalyDetectorInput{
		MetricMathAnomalyDetector: in.MetricMathAnomalyDetector, SingleMetricAnomalyDetector: in.SingleMetricAnomalyDetector,
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
		if id.same(stored) {
			if err := f.failer.Before(); err != nil {
				return nil, err
			}
			f.store.Remove(anomalyDetectorType, identifier)
			return &cloudwatch.DeleteAnomalyDetectorOutput{}, f.failer.After()
		}
	}
	return nil, fault("ResourceNotFoundException")
}
