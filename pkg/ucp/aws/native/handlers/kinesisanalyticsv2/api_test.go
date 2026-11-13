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

package kinesisanalyticsv2

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2"
	"github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/managed-flink/latest/apiv2/"
	ruleStream   = "arn:aws:logs:us-west-2:123456789012:log-group:rules:log-stream:rules"
)

// version returns the current version ID of the application.
func version(ctx context.Context, f *fakeAPI, name string) (int64, error) {
	out, err := f.DescribeApplication(ctx, &kinesisanalyticsv2.DescribeApplicationInput{ApplicationName: aws.String(name)})
	if err != nil {
		return 0, err
	}
	return aws.ToInt64(out.ApplicationDetail.ApplicationVersionId), nil
}

// addOption adds a logging option with stream to app-one at its current version, and returns the option's ID.
func addOption(ctx context.Context, f *fakeAPI, stream string) (string, error) {
	v, err := version(ctx, f, "app-one")
	if err != nil {
		return "", err
	}
	out, err := f.AddApplicationCloudWatchLoggingOption(ctx, &kinesisanalyticsv2.AddApplicationCloudWatchLoggingOptionInput{
		ApplicationName:             aws.String("app-one"),
		CloudWatchLoggingOption:     &types.CloudWatchLoggingOption{LogStreamARN: aws.String(stream)},
		CurrentApplicationVersionId: aws.Int64(v),
	})
	if err != nil {
		return "", err
	}
	ids := withLogStream(out.CloudWatchLoggingOptionDescriptions, stream)
	if len(ids) != 1 {
		return "", fmt.Errorf("the application has logging options %v with the stream", ids)
	}
	return ids[0], nil
}

// change makes a call that changes app-one's logging options with f. It sends version, or the application's current
// version when version is nil, and edit, when set, changes the input's application name and conditional token.
type change func(ctx context.Context, f *fakeAPI, version *int64, edit func(name **string, token **string)) error

// changeRules returns the rules AWS's API reference gives for every call that changes an application's logging
// options, made with call.
func changeRules(call change) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule: "ApplicationName is 1 to 128 characters of [a-zA-Z0-9_.-]",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, newFakeAPI(store), nil, func(name **string, _ **string) { *name = aws.String("app one") })
			},
			Code: "InvalidArgumentException",
		},
		{
			Rule: "CurrentApplicationVersionId is 1 to 999999999",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, newFakeAPI(store), aws.Int64(0), nil)
			},
			Code: "InvalidArgumentException",
		},
		{
			Rule: "ConditionalToken is 1 to 512 characters of [a-zA-Z0-9-_+/=]",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, newFakeAPI(store), nil, func(_ **string, token **string) { *token = aws.String(strings.Repeat("a", 513)) })
			},
			Code: "InvalidArgumentException",
		},
		{
			Rule: "a change sent with a version ID that isn't the application's current one fails",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, newFakeAPI(store), aws.Int64(999), nil)
			},
			Code: "ConcurrentModificationException",
		},
		{
			Rule: "the application exists",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return call(ctx, newFakeAPI(store), aws.Int64(1), func(name **string, _ **string) { *name = aws.String("app-missing") })
			},
			Code: "ResourceNotFoundException",
		},
		{
			Rule: "AWS sets the application's version ID up each time it changes the logging options",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				f := newFakeAPI(store)
				before, err := version(ctx, f, "app-one")
				if err != nil {
					return err
				}
				if err := call(ctx, f, nil, nil); err != nil {
					return err
				}
				after, err := version(ctx, f, "app-one")
				if err != nil {
					return err
				}
				if after <= before {
					return fmt.Errorf("the version ID went from %d to %d", before, after)
				}
				return nil
			},
		},
	}
}

// currentVersion returns version, or the current version of app-one when version is nil.
func currentVersion(ctx context.Context, f *fakeAPI, v *int64) (*int64, error) {
	if v != nil {
		return v, nil
	}
	current, err := version(ctx, f, "app-one")
	return aws.Int64(current), err
}

// add adds a logging option to app-one, for changeRules.
func add(ctx context.Context, f *fakeAPI, v *int64, edit func(name **string, token **string)) error {
	v, err := currentVersion(ctx, f, v)
	if err != nil {
		return err
	}
	in := &kinesisanalyticsv2.AddApplicationCloudWatchLoggingOptionInput{
		ApplicationName:             aws.String("app-one"),
		CloudWatchLoggingOption:     &types.CloudWatchLoggingOption{LogStreamARN: aws.String(ruleStream)},
		CurrentApplicationVersionId: v,
	}
	if edit != nil {
		edit(&in.ApplicationName, &in.ConditionalToken)
	}
	_, err = f.AddApplicationCloudWatchLoggingOption(ctx, in)
	return err
}

// deleteOption adds a logging option to app-one, then deletes it, for changeRules.
func deleteOption(ctx context.Context, f *fakeAPI, v *int64, edit func(name **string, token **string)) error {
	id, err := addOption(ctx, f, ruleStream)
	if err != nil {
		return err
	}
	v, err = currentVersion(ctx, f, v)
	if err != nil {
		return err
	}
	in := &kinesisanalyticsv2.DeleteApplicationCloudWatchLoggingOptionInput{
		ApplicationName:             aws.String("app-one"),
		CloudWatchLoggingOptionId:   aws.String(id),
		CurrentApplicationVersionId: v,
	}
	if edit != nil {
		edit(&in.ApplicationName, &in.ConditionalToken)
	}
	_, err = f.DeleteApplicationCloudWatchLoggingOption(ctx, in)
	return err
}

// updateOption adds a logging option to app-one, then sends it another log stream, for changeRules.
func updateOption(ctx context.Context, f *fakeAPI, v *int64, edit func(name **string, token **string)) error {
	id, err := addOption(ctx, f, ruleStream)
	if err != nil {
		return err
	}
	v, err = currentVersion(ctx, f, v)
	if err != nil {
		return err
	}
	in := &kinesisanalyticsv2.UpdateApplicationInput{
		ApplicationName:             aws.String("app-one"),
		CurrentApplicationVersionId: v,
		CloudWatchLoggingOptionUpdates: []types.CloudWatchLoggingOptionUpdate{{
			CloudWatchLoggingOptionId: aws.String(id),
			LogStreamARNUpdate:        aws.String(ruleStream + "-2"),
		}},
	}
	if edit != nil {
		edit(&in.ApplicationName, &in.ConditionalToken)
	}
	_, err = f.UpdateApplication(ctx, in)
	return err
}

// changeUnclear is what AWS's docs leave unclear about every call that changes logging options.
const changeUnclear = "The page says to send CurrentApplicationVersionId or ConditionalToken, but not which error AWS " +
	"returns for neither or whether both can be sent. The fake doesn't model ConditionalToken: it checks its limits, " +
	"then fails a change without the current CurrentApplicationVersionId with ConcurrentModificationException. The " +
	"page lists ResourceInUseException without the application statuses it applies to, so the fake accepts a change " +
	"in any status."

// loggingOptionAPI records the Kinesis Analytics v2 calls LoggingOption makes.
var loggingOptionAPI = map[string]nativetest.Call{
	"AddApplicationCloudWatchLoggingOption": {
		Doc: apiReference + "API_AddApplicationCloudWatchLoggingOption.html",
		Rules: append(changeRules(add), nativetest.Rule{
			Rule: "LogStreamARN is 1 to 2048 characters matching arn:.*",
			Doc:  apiReference + "API_CloudWatchLoggingOption.html",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				_, err := addOption(ctx, newFakeAPI(store), "logs/rules")
				return err
			},
			Code: "InvalidArgumentException",
		}, nativetest.Rule{
			Rule: "LogStreamARN's pattern arn:.* allows any character, and its limit counts characters, not bytes",
			Doc:  apiReference + "API_CloudWatchLoggingOption.html",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				_, err := addOption(ctx, newFakeAPI(store), "arn:"+strings.Repeat("界", 2044))
				return err
			},
		}),
		Unclear: changeUnclear + " CloudFormation's page for the type says an application takes one such resource, but " +
			"the API reference states no limit on logging options or an error for a second one, so the fake accepts more.",
	},
	"DeleteApplicationCloudWatchLoggingOption": {
		Doc: apiReference + "API_DeleteApplicationCloudWatchLoggingOption.html",
		Rules: append(changeRules(deleteOption), nativetest.Rule{
			Rule: "CloudWatchLoggingOptionId is 1 to 50 characters of [a-zA-Z0-9_.-]",
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				_, err := newFakeAPI(store).DeleteApplicationCloudWatchLoggingOption(ctx, &kinesisanalyticsv2.DeleteApplicationCloudWatchLoggingOptionInput{
					ApplicationName:             aws.String("app-one"),
					CloudWatchLoggingOptionId:   aws.String(strings.Repeat("1", 51)),
					CurrentApplicationVersionId: aws.Int64(1),
				})
				return err
			},
			Code: "InvalidArgumentException",
		}),
		Unclear: changeUnclear + " The page doesn't name the error for a logging option ID the application doesn't " +
			"have; the fake returns ResourceNotFoundException, and the handler treats it as already deleted.",
	},
	"DescribeApplication": {
		Doc: apiReference + "API_DescribeApplication.html",
		Rules: []nativetest.Rule{
			{
				Rule: "ApplicationName is 1 to 128 characters of [a-zA-Z0-9_.-]",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := version(ctx, newFakeAPI(store), strings.Repeat("a", 129))
					return err
				},
				Code: "InvalidArgumentException",
			},
			{
				Rule: "the application exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := version(ctx, newFakeAPI(store), "app-missing")
					return err
				},
				Code: "ResourceNotFoundException",
			},
		},
	},
	"ListApplications": {
		Doc: apiReference + "API_ListApplications.html",
		Rules: []nativetest.Rule{
			{
				Rule: "Limit is 1 to 50",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).ListApplications(ctx, &kinesisanalyticsv2.ListApplicationsInput{Limit: aws.Int32(51)})
					return err
				},
				Code: "InvalidRequestException",
			},
			{
				Rule: "NextToken is 1 to 128 characters of [a-zA-Z0-9_.-]",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).ListApplications(ctx, &kinesisanalyticsv2.ListApplicationsInput{NextToken: aws.String("")})
					return err
				},
				Code: "InvalidRequestException",
			},
		},
		Unclear: "The page lists only InvalidRequestException, so the fake returns it for a Limit or NextToken outside " +
			"their limits.",
	},
	"UpdateApplication": {
		Doc: apiReference + "API_UpdateApplication.html",
		Rules: append(changeRules(updateOption),
			nativetest.Rule{
				Rule: "CloudWatchLoggingOptionId is 1 to 50 characters of [a-zA-Z0-9_.-]",
				Doc:  apiReference + "API_CloudWatchLoggingOptionUpdate.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).UpdateApplication(ctx, &kinesisanalyticsv2.UpdateApplicationInput{
						ApplicationName:                aws.String("app-one"),
						CurrentApplicationVersionId:    aws.Int64(1),
						CloudWatchLoggingOptionUpdates: []types.CloudWatchLoggingOptionUpdate{{CloudWatchLoggingOptionId: aws.String("1 1"), LogStreamARNUpdate: aws.String(ruleStream)}},
					})
					return err
				},
				Code: "InvalidArgumentException",
			},
			nativetest.Rule{
				Rule: "LogStreamARNUpdate is 1 to 2048 characters matching arn:.*",
				Doc:  apiReference + "API_CloudWatchLoggingOptionUpdate.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := addOption(ctx, f, ruleStream)
					if err != nil {
						return err
					}
					v, err := version(ctx, f, "app-one")
					if err != nil {
						return err
					}
					_, err = f.UpdateApplication(ctx, &kinesisanalyticsv2.UpdateApplicationInput{
						ApplicationName:                aws.String("app-one"),
						CurrentApplicationVersionId:    aws.Int64(v),
						CloudWatchLoggingOptionUpdates: []types.CloudWatchLoggingOptionUpdate{{CloudWatchLoggingOptionId: aws.String(id), LogStreamARNUpdate: aws.String("arn:" + strings.Repeat("a", 2045))}},
					})
					return err
				},
				Code: "InvalidArgumentException",
			},
		),
		Unclear: changeUnclear + " The page says the call updates only existing logging options, but doesn't name the " +
			"error for an ID the application doesn't have; the fake returns ResourceNotFoundException.",
	},
}
