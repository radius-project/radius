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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference     = "https://docs.aws.amazon.com/AmazonCloudWatch/latest/APIReference/"
	singleMetricDocs = apiReference + "API_SingleMetricAnomalyDetector.html"
	metricMathDocs   = apiReference + "API_MetricMathAnomalyDetector.html"
	queryDocs        = apiReference + "API_MetricDataQuery.html"
	configDocs       = apiReference + "API_AnomalyDetectorConfiguration.html"
)

// metricDetector returns a detector of the CPU of an instance, with stat.
func metricDetector(stat string) *types.SingleMetricAnomalyDetector {
	return &types.SingleMetricAnomalyDetector{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []types.Dimension{{Name: aws.String("InstanceId"), Value: aws.String("i-1")}},
		Stat:       aws.String(stat),
	}
}

// metricQuery returns a query of the CPU of an instance, with id and returnData.
func metricQuery(id string, returnData bool) types.MetricDataQuery {
	return types.MetricDataQuery{
		Id:         aws.String(id),
		ReturnData: aws.Bool(returnData),
		MetricStat: &types.MetricStat{
			Metric: &types.Metric{Namespace: aws.String("AWS/EC2"), MetricName: aws.String("CPUUtilization")},
			Period: aws.Int32(300),
			Stat:   aws.String("Average"),
		},
	}
}

// mathDetectorOf returns a metric math detector of queries.
func mathDetectorOf(queries ...types.MetricDataQuery) *types.MetricMathAnomalyDetector {
	return &types.MetricMathAnomalyDetector{MetricDataQueries: queries}
}

// putDetector makes in with a fake CloudWatch API backed by store.
func putDetector(ctx context.Context, store *awsfake.CloudControl, in *cloudwatch.PutAnomalyDetectorInput) error {
	_, err := newFakeAPI(store).PutAnomalyDetector(ctx, in)
	return err
}

// putConfiguration puts a detector of one metric with config.
func putConfiguration(ctx context.Context, store *awsfake.CloudControl, config *types.AnomalyDetectorConfiguration) error {
	return putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{SingleMetricAnomalyDetector: metricDetector("Average"), Configuration: config})
}

func describeDetectors(ctx context.Context, store *awsfake.CloudControl, in *cloudwatch.DescribeAnomalyDetectorsInput) (*cloudwatch.DescribeAnomalyDetectorsOutput, error) {
	return newFakeAPI(store).DescribeAnomalyDetectors(ctx, in)
}

func deleteDetector(ctx context.Context, store *awsfake.CloudControl, in *cloudwatch.DeleteAnomalyDetectorInput) error {
	_, err := newFakeAPI(store).DeleteAnomalyDetector(ctx, in)
	return err
}

// tooManyDimensions returns one dimension more than a metric can have.
func tooManyDimensions() []types.Dimension {
	out := []types.Dimension{}
	for i := range maxDimensions + 1 {
		out = append(out, types.Dimension{Name: aws.String("d" + strconv.Itoa(i)), Value: aws.String("v")})
	}
	return out
}

// singleMetricRulesOf returns the rules CloudWatch's API reference gives for the detector of one metric that a put or
// delete names, for call, which makes such a call with single.
func singleMetricRulesOf(call func(ctx context.Context, store *awsfake.CloudControl, single *types.SingleMetricAnomalyDetector) error) []nativetest.Rule {
	with := func(change func(s *types.SingleMetricAnomalyDetector)) func(ctx context.Context, store *awsfake.CloudControl) error {
		return func(ctx context.Context, store *awsfake.CloudControl) error {
			s := metricDetector("Average")
			change(s)
			return call(ctx, store, s)
		}
	}
	return []nativetest.Rule{
		{
			Rule:  "a metric has at most 30 dimensions",
			Doc:   singleMetricDocs,
			Break: with(func(s *types.SingleMetricAnomalyDetector) { s.Dimensions = tooManyDimensions() }),
			Code:  "ValidationError",
		},
		{
			Rule: "a dimension's Name is 1 to 255 characters and its Value 1 to 1024",
			Doc:  apiReference + "API_Dimension.html",
			Break: with(func(s *types.SingleMetricAnomalyDetector) {
				s.Dimensions[0].Value = aws.String(strings.Repeat("v", 1025))
			}),
			Code: "ValidationError",
		},
		{
			Rule:  "Namespace is 1 to 255 characters and doesn't start with a colon",
			Doc:   singleMetricDocs,
			Break: with(func(s *types.SingleMetricAnomalyDetector) { s.Namespace = aws.String(":AWS/EC2") }),
			Code:  "ValidationError",
		},
		{
			Rule:  "MetricName is 1 to 255 characters",
			Doc:   singleMetricDocs,
			Break: with(func(s *types.SingleMetricAnomalyDetector) { s.MetricName = aws.String(strings.Repeat("é", 256)) }),
			Code:  "ValidationError",
		},
		{
			Rule:  "Stat is at most 50 characters of the documented statistic pattern, such as Average, p90, or TM(10%:90%)",
			Doc:   singleMetricDocs,
			Break: with(func(s *types.SingleMetricAnomalyDetector) { s.Stat = aws.String("Mean") }),
			Code:  "ValidationError",
		},
		{
			Rule:  "AccountId is 1 to 255 characters",
			Doc:   singleMetricDocs,
			Break: with(func(s *types.SingleMetricAnomalyDetector) { s.AccountId = aws.String("") }),
			Code:  "ValidationError",
		},
	}
}

// formRulesOf returns the rules CloudWatch's API reference gives for the forms of a detector a put or delete names, for
// call, which makes a call with the detectors and top-level fields in in.
func formRulesOf(call func(ctx context.Context, store *awsfake.CloudControl, in *cloudwatch.PutAnomalyDetectorInput) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "SingleMetricAnomalyDetector and MetricMathAnomalyDetector can't be combined",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, store, &cloudwatch.PutAnomalyDetectorInput{SingleMetricAnomalyDetector: metricDetector("Average"), MetricMathAnomalyDetector: mathDetectorOf(metricQuery("m1", true))})
			},
			Code: "InvalidParameterCombination",
		},
		{
			Rule: "SingleMetricAnomalyDetector and MetricMathAnomalyDetector can't be combined with the deprecated top-level " +
				"Namespace, MetricName, Dimensions, and Stat",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				//nolint:staticcheck // The rule is about the deprecated fields.
				return call(ctx, store, &cloudwatch.PutAnomalyDetectorInput{SingleMetricAnomalyDetector: metricDetector("Average"), Stat: aws.String("Sum")})
			},
			Code: "InvalidParameterCombination",
		},
	}
}

// accountCheck returns a check that a detector of one metric without AccountId is the current account's: a put with
// the current account's ID and one without it make one detector, which then makes the calls finish.
func accountCheck(finish func(ctx context.Context, store *awsfake.CloudControl) error) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		named := metricDetector("Average")
		named.AccountId = aws.String(fakeAccount)
		for _, s := range []*types.SingleMetricAnomalyDetector{metricDetector("Average"), named} {
			if err := putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{SingleMetricAnomalyDetector: s}); err != nil {
				return err
			}
		}
		if ids := store.List(anomalyDetectorType); len(ids) != 1 {
			return fmt.Errorf("the puts made the detectors %v", ids)
		}
		return finish(ctx, store)
	}
}

// codeNote is what CloudWatch's pages leave unclear about the error codes of the limits the fake enforces.
const codeNote = "The pages don't say which error code each limit gets. For lengths, patterns, list sizes, and enumerations " +
	"the fake returns ValidationError, the code CloudWatch's Common Errors page gives for input that doesn't meet a " +
	"format or constraint; for other bad values it returns InvalidParameterValue, the call's code for a bad or " +
	"out-of-range value; and for a missing field, MissingParameter. The fake doesn't check that dimension names and " +
	"values are ASCII with a non-whitespace character, which the Dimension page states without an error code."

// anomalyDetectorAPI records the CloudWatch and STS calls AnomalyDetector makes.
var anomalyDetectorAPI = map[string]nativetest.Call{
	"PutAnomalyDetector": {
		Doc: apiReference + "API_PutAnomalyDetector.html",
		Rules: append(append(
			formRulesOf(putDetector),
			singleMetricRulesOf(func(ctx context.Context, store *awsfake.CloudControl, s *types.SingleMetricAnomalyDetector) error {
				return putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{SingleMetricAnomalyDetector: s})
			})...),
			nativetest.Rule{
				Rule: "a metric math detector has exactly one query with ReturnData true, the expression the detector watches",
				Doc:  metricMathDocs,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{MetricMathAnomalyDetector: mathDetectorOf(metricQuery("m1", true), metricQuery("m2", true))})
				},
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "a query sets Expression or MetricStat, not both",
				Doc:  queryDocs,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					q := metricQuery("m1", true)
					q.Expression = aws.String("m1 * 2")
					return putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{MetricMathAnomalyDetector: mathDetectorOf(q)})
				},
				Code: "InvalidParameterCombination",
			},
			nativetest.Rule{
				Rule: "a query's Id has letters, numbers, and underscores, and starts with a lowercase letter",
				Doc:  queryDocs,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{MetricMathAnomalyDetector: mathDetectorOf(metricQuery("M1", true))})
				},
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "MetricStat needs Metric, Period, and Stat",
				Doc:  apiReference + "API_MetricStat.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					q := metricQuery("m1", true)
					q.MetricStat.Stat = nil
					return putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{MetricMathAnomalyDetector: mathDetectorOf(q)})
				},
				Code: "MissingParameter",
			},
			nativetest.Rule{
				Rule: "MetricStat's Period is at least 1",
				Doc:  apiReference + "API_MetricStat.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					q := metricQuery("m1", true)
					q.MetricStat.Period = aws.Int32(0)
					return putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{MetricMathAnomalyDetector: mathDetectorOf(q)})
				},
				Code: "ValidationError",
			},
			nativetest.Rule{
				Rule: "Configuration has at most 10 excluded time ranges",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
					config := &types.AnomalyDetectorConfiguration{}
					for i := range maxExcludedRanges + 1 {
						start := day.AddDate(0, 0, 2*i)
						config.ExcludedTimeRanges = append(config.ExcludedTimeRanges, types.Range{StartTime: aws.Time(start), EndTime: aws.Time(start.Add(time.Hour))})
					}
					return putConfiguration(ctx, store, config)
				},
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "an excluded time range needs StartTime and EndTime",
				Doc:  apiReference + "API_Range.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
					return putConfiguration(ctx, store, &types.AnomalyDetectorConfiguration{ExcludedTimeRanges: []types.Range{{StartTime: aws.Time(start)}}})
				},
				Code: "MissingParameter",
			},
			nativetest.Rule{
				Rule: "MetricTimezone is at most 50 characters",
				Doc:  configDocs,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putConfiguration(ctx, store, &types.AnomalyDetectorConfiguration{MetricTimezone: aws.String("America/" + strings.Repeat("x", 43))})
				},
				Code: "ValidationError",
			},
			nativetest.Rule{
				Rule: "MetricTimezone is the name of a time zone in the tz database, such as America/New_York",
				Doc:  configDocs,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putConfiguration(ctx, store, &types.AnomalyDetectorConfiguration{MetricTimezone: aws.String("Mars/Olympus_Mons")})
				},
				Code: "InvalidParameterValue",
			},
			nativetest.Rule{
				Rule: "AdaptationSpeed is FAST, NORMAL, or SLOW, and OutlierFilteringStrength is MINIMAL, MODERATE, or STRICT",
				Doc:  configDocs,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putConfiguration(ctx, store, &types.AnomalyDetectorConfiguration{AdaptationSpeed: "QUICK"})
				},
				Code: "ValidationError",
			},
			nativetest.Rule{
				Rule: "lengths count characters, so a MetricName of 255 non-ASCII characters is within its limit",
				Doc:  singleMetricDocs,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					s := metricDetector("Average")
					s.MetricName = aws.String(strings.Repeat("é", 255))
					return putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{SingleMetricAnomalyDetector: s})
				},
			},
			nativetest.Rule{
				Rule:  "a detector of one metric without AccountId is in the current account",
				Doc:   singleMetricDocs,
				Check: accountCheck(func(context.Context, *awsfake.CloudControl) error { return nil }),
			},
		),
		Unclear: codeNote + " The page doesn't say what a put of a detector AWS already has does. The fake replaces its " +
			"Configuration and MetricCharacteristics, and clears its configuration when the put has none; the handler " +
			"sends an empty configuration to clear one, so it doesn't depend on that. The page lists " +
			"LimitExceededException without the limit; the fake never returns it. MetricDataQuery documents ReturnData " +
			"defaulting to true only for GetMetricData; the fake treats a query without ReturnData as true, as AWS " +
			"returned it in recorded responses. The pages don't say whether a metric math detector needs a query with " +
			"an Expression; the fake accepts one whose only query is a MetricStat. AnomalyDetectorConfiguration gives " +
			"NORMAL and MODERATE as the defaults of AdaptationSpeed and OutlierFilteringStrength, but not whether a read " +
			"returns them; AWS's recorded responses don't, and neither does the fake. A put that names no metric gets " +
			"MissingParameter from the fake; the page marks no field required, and the fake accepts a detector of one " +
			"metric without MetricName or Stat. The page doesn't say whether an " +
			"excluded range's StartTime must come before its EndTime; the fake doesn't check.",
	},
	"DescribeAnomalyDetectors": {
		Doc: apiReference + "API_DescribeAnomalyDetectors.html",
		Rules: []nativetest.Rule{
			{
				Rule: "AnomalyDetectorTypes left empty means SINGLE_METRIC",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{SingleMetricAnomalyDetector: metricDetector("Average")}); err != nil {
						return err
					}
					if err := putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{MetricMathAnomalyDetector: mathDetectorOf(metricQuery("m1", true))}); err != nil {
						return err
					}
					out, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{})
					if err != nil {
						return err
					}
					if len(out.AnomalyDetectors) != 1 || out.AnomalyDetectors[0].SingleMetricAnomalyDetector == nil {
						return fmt.Errorf("the describe returned %d detectors", len(out.AnomalyDetectors))
					}
					return nil
				},
			},
			{
				Rule: "a Dimensions filter returns the detectors of every metric that has those dimensions, including " +
					"metrics with more",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					more := metricDetector("Average")
					more.Dimensions = append(more.Dimensions, types.Dimension{Name: aws.String("AutoScalingGroupName"), Value: aws.String("asg")})
					if err := putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{SingleMetricAnomalyDetector: more}); err != nil {
						return err
					}
					out, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{Dimensions: metricDetector("Average").Dimensions})
					if err != nil {
						return err
					}
					if len(out.AnomalyDetectors) != 1 {
						return fmt.Errorf("the describe returned %d detectors", len(out.AnomalyDetectors))
					}
					return nil
				},
			},
			{
				Rule: "MaxResults limits a page, and the NextToken it returns gets the next",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					for _, stat := range []string{"Average", "Sum"} {
						if err := putDetector(ctx, store, &cloudwatch.PutAnomalyDetectorInput{SingleMetricAnomalyDetector: metricDetector(stat)}); err != nil {
							return err
						}
					}
					first, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{MaxResults: aws.Int32(1)})
					if err != nil {
						return err
					}
					if len(first.AnomalyDetectors) != 1 || first.NextToken == nil {
						return fmt.Errorf("the first page has %d detectors", len(first.AnomalyDetectors))
					}
					next, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{MaxResults: aws.Int32(1), NextToken: first.NextToken})
					if err != nil {
						return err
					}
					if len(next.AnomalyDetectors) != 1 || next.NextToken != nil {
						return fmt.Errorf("the next page has %d detectors", len(next.AnomalyDetectors))
					}
					return nil
				},
			},
			{
				Rule: "AnomalyDetectorIds can't be combined with Namespace, MetricName, Dimensions, or AnomalyDetectorTypes",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{AnomalyDetectorIds: []string{"detector"}, Namespace: aws.String("AWS/EC2")})
					return err
				},
				Code: "InvalidParameterCombination",
			},
			{
				Rule: "AnomalyDetectorTypes has at most 2 items, each SINGLE_METRIC or METRIC_MATH",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{AnomalyDetectorTypes: []types.AnomalyDetectorType{"COMPOSITE"}})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "Dimensions has at most 30 items",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{Dimensions: tooManyDimensions()})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "Namespace is 1 to 255 characters and doesn't start with a colon, and MetricName is 1 to 255 characters",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{MetricName: aws.String("")})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "MaxResults is at least 1",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{MaxResults: aws.Int32(0)})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "MaxResults is at most 100",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{MaxResults: aws.Int32(maxResults + 1)})
					return err
				},
				Code: "InvalidParameterValue",
			},
			{
				Rule: "NextToken is a token a previous call returned",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := describeDetectors(ctx, store, &cloudwatch.DescribeAnomalyDetectorsInput{NextToken: aws.String("not-a-token")})
					return err
				},
				Code: "InvalidNextToken",
			},
		},
		Unclear: codeNote + " The page doesn't give the default page size; the fake returns every detector on one page " +
			"unless MaxResults or its pageSize is set. The fake doesn't filter by AnomalyDetectorIds, which the handler " +
			"never sends. It leaves out the deprecated top-level Namespace, MetricName, Dimensions, and Stat that AWS " +
			"also returns, which the handler never reads.",
	},
	"DeleteAnomalyDetector": {
		Doc: apiReference + "API_DeleteAnomalyDetector.html",
		Rules: append(append(
			formRulesOf(func(ctx context.Context, store *awsfake.CloudControl, in *cloudwatch.PutAnomalyDetectorInput) error {
				return deleteDetector(ctx, store, &cloudwatch.DeleteAnomalyDetectorInput{
					SingleMetricAnomalyDetector: in.SingleMetricAnomalyDetector, MetricMathAnomalyDetector: in.MetricMathAnomalyDetector,
					//nolint:staticcheck // The form rules cover the deprecated fields.
					Namespace: in.Namespace, MetricName: in.MetricName, Dimensions: in.Dimensions, Stat: in.Stat,
				})
			}),
			singleMetricRulesOf(func(ctx context.Context, store *awsfake.CloudControl, s *types.SingleMetricAnomalyDetector) error {
				return deleteDetector(ctx, store, &cloudwatch.DeleteAnomalyDetectorInput{SingleMetricAnomalyDetector: s})
			})...),
			nativetest.Rule{
				Rule: "the detector exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return deleteDetector(ctx, store, &cloudwatch.DeleteAnomalyDetectorInput{SingleMetricAnomalyDetector: metricDetector("Average")})
				},
				Code: "ResourceNotFoundException",
			},
			nativetest.Rule{
				Rule: "a detector of one metric without AccountId is in the current account, so a delete without it " +
					"deletes the detector a put with the current account's ID made",
				Doc: singleMetricDocs,
				Check: accountCheck(func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := deleteDetector(ctx, store, &cloudwatch.DeleteAnomalyDetectorInput{SingleMetricAnomalyDetector: metricDetector("Average")}); err != nil {
						return err
					}
					if ids := store.List(anomalyDetectorType); len(ids) != 0 {
						return fmt.Errorf("the delete left %v", ids)
					}
					return nil
				}),
			},
		),
		Unclear: codeNote + " The page doesn't say which error a delete that names no detector gets; the fake returns " +
			"ResourceNotFoundException. The fake doesn't delete by AnomalyDetectorId, which the handler never sends.",
	},
	"GetCallerIdentity": {
		Doc: "https://docs.aws.amazon.com/STS/latest/APIReference/API_GetCallerIdentity.html",
		Rules: []nativetest.Rule{
			{
				Rule: "the call needs no permissions and returns the account that owns the caller",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					out, err := newFakeAPI(store).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
					if err != nil {
						return err
					}
					if aws.ToString(out.Account) != fakeAccount {
						return fmt.Errorf("the call returned the account %q", aws.ToString(out.Account))
					}
					return nil
				},
			},
		},
		Unclear: "The page says a policy that denies sts:GetCallerIdentity doesn't stop the call. The handler's tests " +
			"still make the fake return AccessDenied, to stand for any STS failure, such as expired credentials.",
	},
}
