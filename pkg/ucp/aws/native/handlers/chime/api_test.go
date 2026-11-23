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

package chime

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice"
	"github.com/aws/aws-sdk-go-v2/service/chimesdkvoice/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/chime-sdk/latest/APIReference/"

	// validationNote is what the record of each Put says about AWS's validation errors.
	validationNote = "The page lists BadRequestException for parameters that don't match the service's restrictions, " +
		"but AWS answers a value outside a limit of its API model, such as a range or a list's length, with " +
		"ValidationException, which no page lists and the SDK doesn't model, so its fault is unknown. testdata/errors.json " +
		"has AWS's recorded answers, and the fake returns the same codes. "

	// staleNote is what the record of each Get says about reads right after a change.
	staleNote = "The page doesn't say when a change shows. In live calls, a Get right after a Put or a delete returned the " +
		"earlier settings, or none for settings the Put created, for about a second; the fake returns them for two reads. "
)

func doc(operation string) string {
	return apiReference + "API_voice-chime_" + operation + ".html"
}

// voiceConnectorRule is the rule of every call that names a Voice Connector: AWS has it.
func voiceConnectorRule(call func(ctx context.Context, api *fakeAPI) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "the Voice Connector exists",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			return call(ctx, newFakeAPI(store))
		},
		Code: notFoundCode,
	}
}

// putStreaming sends a Put of settings for testVC to a fake backed by store.
func putStreaming(ctx context.Context, store *awsfake.CloudControl, settings *types.StreamingConfiguration) error {
	_, err := newFakeAPI(store).PutVoiceConnectorStreamingConfiguration(ctx, &chimesdkvoice.PutVoiceConnectorStreamingConfigurationInput{VoiceConnectorId: aws.String(testVC), StreamingConfiguration: settings})
	return err
}

// putOrigination sends a Put of settings for testVC to a fake backed by store.
func putOrigination(ctx context.Context, store *awsfake.CloudControl, settings *types.Origination) error {
	_, err := newFakeAPI(store).PutVoiceConnectorOrigination(ctx, &chimesdkvoice.PutVoiceConnectorOriginationInput{VoiceConnectorId: aws.String(testVC), Origination: settings})
	return err
}

// putTermination sends a Put of settings for testVC to a fake backed by store, and returns what it applied.
func putTermination(ctx context.Context, store *awsfake.CloudControl, settings *types.Termination) (*types.Termination, error) {
	out, err := newFakeAPI(store).PutVoiceConnectorTermination(ctx, &chimesdkvoice.PutVoiceConnectorTerminationInput{VoiceConnectorId: aws.String(testVC), Termination: settings})
	if err != nil {
		return nil, err
	}
	return out.Termination, nil
}

// sdkRoute returns an origination route that AWS takes, changed by change.
func sdkRoute(change func(r *types.OriginationRoute)) types.OriginationRoute {
	r := types.OriginationRoute{Host: aws.String("sip.example.com"), Port: aws.Int32(5060), Protocol: types.OriginationRouteProtocolUdp, Priority: aws.Int32(1), Weight: aws.Int32(1)}
	if change != nil {
		change(&r)
	}
	return r
}

// sdkTermination returns termination settings that AWS takes, changed by change.
func sdkTermination(change func(t *types.Termination)) *types.Termination {
	t := &types.Termination{CallingRegions: []string{"US"}, CidrAllowedList: []string{"203.0.113.0/27"}}
	if change != nil {
		change(t)
	}
	return t
}

// getVoiceConnectorCall records GetVoiceConnector, which every handler calls.
var getVoiceConnectorCall = nativetest.Call{
	Doc: doc("GetVoiceConnector"),
	Rules: []nativetest.Rule{
		voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
			_, err := api.GetVoiceConnector(ctx, &chimesdkvoice.GetVoiceConnectorInput{VoiceConnectorId: aws.String(missingVC)})
			return err
		}),
	},
	Unclear: "The page gives the ID's pattern as .*\\S.*, but AWS answers an ID that isn't 21 or 22 lowercase letters and " +
		"digits, or a UUID, with ValidationException, which the page doesn't list; the fake does the same.",
}

// loggingAPI records the Amazon Chime SDK Voice calls LoggingConfiguration makes.
var loggingAPI = map[string]nativetest.Call{
	"GetVoiceConnector": getVoiceConnectorCall,
	"GetVoiceConnectorLoggingConfiguration": {
		Doc: doc("GetVoiceConnectorLoggingConfiguration"),
		Unclear: staleNote + "The page doesn't say what it returns for a Voice Connector nobody set, or one AWS doesn't " +
			"have. In live calls it returned both logs off for both, without an error, so the handler checks the Voice " +
			"Connector with GetVoiceConnector first; the fake does the same.",
	},
	"PutVoiceConnectorLoggingConfiguration": {
		Doc: doc("PutVoiceConnectorLoggingConfiguration"),
		Rules: []nativetest.Rule{
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				_, err := api.PutVoiceConnectorLoggingConfiguration(ctx, &chimesdkvoice.PutVoiceConnectorLoggingConfigurationInput{VoiceConnectorId: aws.String(missingVC), LoggingConfiguration: &types.LoggingConfiguration{EnableSIPLogs: aws.Bool(true)}})
				return err
			}),
		},
		Unclear: "The page doesn't say what a Put does to a log it leaves out. In live calls it turned the log off, and " +
			"returned only the logs it was sent; the fake does the same. The SDK rejects a call without " +
			"LoggingConfiguration before it sends it, and so does the fake. The page doesn't name the permissions it " +
			"needs to deliver the logs. A recording with only the schema's permissions found that AWS checks the " +
			"caller for logs:CreateLogGroup, logs:DescribeLogGroups, logs:CreateLogDelivery, logs:GetLogDelivery, " +
			"logs:ListLogDeliveries, logs:DeleteLogDelivery, and logs:PutResourcePolicy.",
	},
}

// streamingAPI records the Amazon Chime SDK Voice calls StreamingConfiguration makes.
var streamingAPI = map[string]nativetest.Call{
	"GetVoiceConnectorStreamingConfiguration": {
		Doc: doc("GetVoiceConnectorStreamingConfiguration"),
		Rules: []nativetest.Rule{
			{
				Rule: "the Voice Connector has streaming settings",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetVoiceConnectorStreamingConfiguration(ctx, &chimesdkvoice.GetVoiceConnectorStreamingConfigurationInput{VoiceConnectorId: aws.String(testVC)})
					return err
				},
				Code: notFoundCode,
			},
		},
		Unclear: staleNote,
	},
	"PutVoiceConnectorStreamingConfiguration": {
		Doc: doc("PutVoiceConnectorStreamingConfiguration"),
		Rules: []nativetest.Rule{
			{
				Rule: "DataRetentionInHours is at least 0",
				Doc:  doc("StreamingConfiguration"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putStreaming(ctx, store, &types.StreamingConfiguration{DataRetentionInHours: aws.Int32(-1), Disabled: aws.Bool(false)})
				},
				Code: validationCode,
			},
			{
				Rule: "StreamingNotificationTargets has 1 to 3 items",
				Doc:  doc("StreamingConfiguration"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					target := types.StreamingNotificationTarget{NotificationTarget: types.NotificationTargetSns}
					return putStreaming(ctx, store, &types.StreamingConfiguration{DataRetentionInHours: aws.Int32(1), Disabled: aws.Bool(false), StreamingNotificationTargets: []types.StreamingNotificationTarget{target, target, target, target}})
				},
				Code: validationCode,
			},
			{
				Rule: "NotificationTarget is EventBridge, SNS, or SQS",
				Doc:  doc("StreamingNotificationTarget"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putStreaming(ctx, store, &types.StreamingConfiguration{DataRetentionInHours: aws.Int32(1), Disabled: aws.Bool(false), StreamingNotificationTargets: []types.StreamingNotificationTarget{{NotificationTarget: "Bogus"}}})
				},
				Code: validationCode,
			},
			{
				Rule: "ConfigurationArn matches ^arn[\\/\\:\\-\\_\\.a-zA-Z0-9]+$",
				Doc:  doc("MediaInsightsConfiguration"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putStreaming(ctx, store, &types.StreamingConfiguration{DataRetentionInHours: aws.Int32(1), Disabled: aws.Bool(false), MediaInsightsConfiguration: &types.MediaInsightsConfiguration{ConfigurationArn: aws.String("bogus")}})
				},
				Code: accessDeniedCode,
			},
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				_, err := api.PutVoiceConnectorStreamingConfiguration(ctx, &chimesdkvoice.PutVoiceConnectorStreamingConfigurationInput{VoiceConnectorId: aws.String(missingVC), StreamingConfiguration: &types.StreamingConfiguration{DataRetentionInHours: aws.Int32(1), Disabled: aws.Bool(false)}})
				return err
			}),
		},
		Unclear: validationNote + "The page doesn't say what AWS does when a Put leaves out StreamingNotificationTargets. " +
			"In live calls it set them to EventBridge, even after a Put that set SNS; the fake does the same. A " +
			"MediaInsightsConfiguration ConfigurationArn that doesn't match its pattern got AccessDeniedException, not " +
			"a validation error, because AWS authorizes the call on that ARN; the fake does the same. The SDK rejects a call " +
			"without DataRetentionInHours or Disabled before it sends it, and so does the fake. The page doesn't name " +
			"the permissions a notification target needs: AWS checks the caller for sns:CreateTopic for SNS and " +
			"sqs:CreateQueue for SQS, and creates a topic or queue named ChimeVoiceConnector-Streaming, which stays " +
			"after a delete. With a MediaInsightsConfiguration, AWS checks the caller for " +
			"chime:GetMediaInsightsPipelineConfiguration, and reports Disabled as false when the Put leaves it out; " +
			"the fake does the same.",
	},
	"DeleteVoiceConnectorStreamingConfiguration": {
		Doc: doc("DeleteVoiceConnectorStreamingConfiguration"),
		Rules: []nativetest.Rule{
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				_, err := api.DeleteVoiceConnectorStreamingConfiguration(ctx, &chimesdkvoice.DeleteVoiceConnectorStreamingConfigurationInput{VoiceConnectorId: aws.String(missingVC)})
				return err
			}),
		},
		Unclear: "The page doesn't say what a delete does for a Voice Connector with no streaming settings. In live " +
			"calls it succeeded; the fake does the same.",
	},
}

// originationAPI records the Amazon Chime SDK Voice calls Origination makes.
var originationAPI = map[string]nativetest.Call{
	"GetVoiceConnectorOrigination": {
		Doc: doc("GetVoiceConnectorOrigination"),
		Rules: []nativetest.Rule{
			{
				Rule: "the Voice Connector has origination settings",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetVoiceConnectorOrigination(ctx, &chimesdkvoice.GetVoiceConnectorOriginationInput{VoiceConnectorId: aws.String(testVC)})
					return err
				},
				Code: notFoundCode,
			},
		},
		Unclear: staleNote,
	},
	"PutVoiceConnectorOrigination": {
		Doc: doc("PutVoiceConnectorOrigination"),
		Rules: []nativetest.Rule{
			{
				Rule: "the settings set Routes or Disabled",
				Doc:  doc("Origination"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putOrigination(ctx, store, &types.Origination{})
				},
				Code: badRequestCode,
			},
			{
				Rule: "a Voice Connector has at most ten origination routes",
				Doc:  doc("OriginationRoute"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					routes := []types.OriginationRoute{}
					for range maxRoutes + 1 {
						routes = append(routes, sdkRoute(nil))
					}
					return putOrigination(ctx, store, &types.Origination{Routes: routes})
				},
				Code: badRequestCode,
			},
			{
				Rule: "a route sets at least one parameter",
				Doc:  doc("OriginationRoute"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putOrigination(ctx, store, &types.Origination{Routes: []types.OriginationRoute{{}}})
				},
				Code: badRequestCode,
			},
			{
				Rule: "Port is from 0 to 65535",
				Doc:  doc("OriginationRoute"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putOrigination(ctx, store, &types.Origination{Routes: []types.OriginationRoute{sdkRoute(func(r *types.OriginationRoute) { r.Port = aws.Int32(65536) })}})
				},
				Code: validationCode,
			},
			{
				Rule: "Priority is from 1 to 100",
				Doc:  doc("OriginationRoute"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putOrigination(ctx, store, &types.Origination{Routes: []types.OriginationRoute{sdkRoute(func(r *types.OriginationRoute) { r.Priority = aws.Int32(101) })}})
				},
				Code: validationCode,
			},
			{
				Rule: "Weight is from 1 to 100",
				Doc:  doc("OriginationRoute"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putOrigination(ctx, store, &types.Origination{Routes: []types.OriginationRoute{sdkRoute(func(r *types.OriginationRoute) { r.Weight = aws.Int32(0) })}})
				},
				Code: validationCode,
			},
			{
				Rule: "Protocol is TCP or UDP",
				Doc:  doc("OriginationRoute"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putOrigination(ctx, store, &types.Origination{Routes: []types.OriginationRoute{sdkRoute(func(r *types.OriginationRoute) { r.Protocol = "SCTP" })}})
				},
				Code: validationCode,
			},
			{
				Rule: "an IP address used for Host is publicly routable",
				Doc:  "https://docs.aws.amazon.com/chime-sdk/latest/ag/edit-voicecon.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return putOrigination(ctx, store, &types.Origination{Routes: []types.OriginationRoute{sdkRoute(func(r *types.OriginationRoute) { r.Host = aws.String("10.0.0.10") })}})
				},
				Code: badRequestCode,
			},
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				_, err := api.PutVoiceConnectorOrigination(ctx, &chimesdkvoice.PutVoiceConnectorOriginationInput{VoiceConnectorId: aws.String(missingVC), Origination: &types.Origination{Routes: []types.OriginationRoute{sdkRoute(nil)}}})
				return err
			}),
		},
		Unclear: validationNote + "The Origination page says Routes takes up to 20 routes and that Disabled alone is " +
			"enough, but the OriginationRoute page gives a limit of ten, and in live calls AWS rejected 11 routes, and " +
			"settings without Routes, with BadRequestException. It also rejected a route without Host, Priority, or " +
			"Weight, though the page marks them optional. The fake does the same. The page says Port defaults to " +
			"5060, but AWS reports a port the Put left out as 0; the fake does the same. The admin guide says an IP " +
			"address used for Host must be publicly routable, without naming the error; AWS answered one from a " +
			"documentation range with BadRequestException, and the fake does the same for private, loopback, " +
			"link-local, multicast, and documentation addresses.",
	},
	"DeleteVoiceConnectorOrigination": {
		Doc: doc("DeleteVoiceConnectorOrigination"),
		Rules: []nativetest.Rule{
			{
				Rule: "the Voice Connector has no emergency calling configuration",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeleteVoiceConnectorOrigination(ctx, &chimesdkvoice.DeleteVoiceConnectorOriginationInput{VoiceConnectorId: aws.String(emergencyVC)})
					return err
				},
				Code: badRequestCode,
			},
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				_, err := api.DeleteVoiceConnectorOrigination(ctx, &chimesdkvoice.DeleteVoiceConnectorOriginationInput{VoiceConnectorId: aws.String(missingVC)})
				return err
			}),
		},
		Unclear: "The page says to delete the emergency calling configuration first, without naming the error; the fake " +
			"returns BadRequestException, the page's error for a request that doesn't match the service's " +
			"restrictions. The page doesn't say what a delete does for a Voice Connector with no origination settings. " +
			"In live calls it succeeded; the fake does the same.",
	},
}

// terminationAPI records the Amazon Chime SDK Voice calls Termination makes.
var terminationAPI = map[string]nativetest.Call{
	"GetVoiceConnectorTermination": {
		Doc: doc("GetVoiceConnectorTermination"),
		Rules: []nativetest.Rule{
			{
				Rule: "the Voice Connector has termination settings",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).GetVoiceConnectorTermination(ctx, &chimesdkvoice.GetVoiceConnectorTerminationInput{VoiceConnectorId: aws.String(testVC)})
					return err
				},
				Code: notFoundCode,
			},
		},
		Unclear: staleNote,
	},
	"PutVoiceConnectorTermination": {
		Doc: doc("PutVoiceConnectorTermination"),
		Rules: []nativetest.Rule{
			{
				Rule: "CallingRegions is required",
				Doc:  doc("Termination"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := putTermination(ctx, store, sdkTermination(func(t *types.Termination) { t.CallingRegions = nil }))
					return err
				},
				Code: badRequestCode,
			},
			{
				Rule: "CpsLimit is at least 1",
				Doc:  doc("Termination"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := putTermination(ctx, store, sdkTermination(func(t *types.Termination) { t.CpsLimit = aws.Int32(0) }))
					return err
				},
				Code: validationCode,
			},
			{
				Rule: "CpsLimit is at most the account's service quota",
				Doc:  doc("Termination"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := putTermination(ctx, store, sdkTermination(func(t *types.Termination) { t.CpsLimit = aws.Int32(cpsQuota + 1) }))
					return err
				},
				Code: badRequestCode,
			},
			{
				Rule: "CpsLimit is 1 when left out",
				Doc:  doc("Termination"),
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					applied, err := putTermination(ctx, store, sdkTermination(nil))
					if err != nil {
						return err
					}
					if aws.ToInt32(applied.CpsLimit) != 1 {
						return fmt.Errorf("CpsLimit is %d", aws.ToInt32(applied.CpsLimit))
					}
					return nil
				},
			},
			{
				Rule: "DefaultPhoneNumber matches ^\\+?[1-9]\\d{1,14}$",
				Doc:  doc("Termination"),
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := putTermination(ctx, store, sdkTermination(func(t *types.Termination) { t.DefaultPhoneNumber = aws.String("abc") }))
					return err
				},
				Code: validationCode,
			},
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				_, err := api.PutVoiceConnectorTermination(ctx, &chimesdkvoice.PutVoiceConnectorTerminationInput{VoiceConnectorId: aws.String(missingVC), Termination: sdkTermination(nil)})
				return err
			}),
		},
		Unclear: validationNote + "The page marks CidrAllowedList optional, but in live calls AWS rejected settings " +
			"without it, a CIDR block wider than /27, and a private one, with BadRequestException, and a country it " +
			"doesn't serve. The fake does the same, and takes US and CA. The page doesn't say what Disabled is when " +
			"left out; AWS reported false, and so does the fake. The fake's quota is 1 call per second, the default " +
			"quota of the recording account. The page doesn't say DefaultPhoneNumber must be a phone number of the " +
			"Voice Connector, but AWS rejected another with BadRequestException; the fake does the same. Assigning a " +
			"number costs money, so no recording shows whether GetVoiceConnectorTermination returns it: the fake leaves " +
			"it out, and the schema makes it write-only.",
	},
	"DeleteVoiceConnectorTermination": {
		Doc: doc("DeleteVoiceConnectorTermination"),
		Rules: []nativetest.Rule{
			{
				Rule: "the Voice Connector has no emergency calling configuration",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeleteVoiceConnectorTermination(ctx, &chimesdkvoice.DeleteVoiceConnectorTerminationInput{VoiceConnectorId: aws.String(emergencyVC)})
					return err
				},
				Code: badRequestCode,
			},
			voiceConnectorRule(func(ctx context.Context, api *fakeAPI) error {
				_, err := api.DeleteVoiceConnectorTermination(ctx, &chimesdkvoice.DeleteVoiceConnectorTerminationInput{VoiceConnectorId: aws.String(missingVC)})
				return err
			}),
		},
		Unclear: "The page says to delete the emergency calling configuration first, without naming the error; the fake " +
			"returns BadRequestException, the page's error for a request that doesn't match the service's " +
			"restrictions. The page doesn't say what a delete does for a Voice Connector with no termination settings, " +
			"or to its SIP credentials. In live calls it succeeded, and the credentials stayed; the fake does the same.",
	},
}
