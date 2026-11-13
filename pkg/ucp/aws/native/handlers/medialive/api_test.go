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

package medialive

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/medialive"
	"github.com/aws/aws-sdk-go-v2/service/medialive/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// MediaLive's API reference names each error by its HTTP status and response model, and the SDK reports them by code:
// 400 InvalidRequest is BadRequestException, 404 ResourceNotFound is NotFoundException, 409 ResourceConflict is
// ConflictException, and 422 ChannelConfigurationValidationError is UnprocessableEntityException.
const (
	apiReference  = "https://docs.aws.amazon.com/medialive/latest/apireference/"
	channelsPage  = apiReference + "channels.html"
	channelPage   = apiReference + "channels-channelid.html"
	classPage     = apiReference + "channels-channelid-channelclass.html"
	tagsPage      = apiReference + "tags-resource-arn.html"
	userGuide     = "https://docs.aws.amazon.com/medialive/latest/ug/"
	editingPage   = userGuide + "editing-deleting-channel.html"
	classChange   = userGuide + "pipeline-redundancy-change.html"
	tagRules      = userGuide + "tagging.html"
	testRoleArn   = "arn:aws:iam::123456789012:role/medialive"
	errorsUnclear = "The page lists each error by HTTP status without saying which one a rule gets. "
)

// channelInput returns a valid request for a small channel of the class, with a destination that has the addresses.
func channelInput(class types.ChannelClass, urls ...string) *medialive.CreateChannelInput {
	settings := []types.OutputDestinationSettings{}
	for _, u := range urls {
		settings = append(settings, types.OutputDestinationSettings{Url: aws.String(u)})
	}
	return &medialive.CreateChannelInput{
		Name:         aws.String("app-channel"),
		ChannelClass: class,
		RoleArn:      aws.String(testRoleArn),
		RequestId:    aws.String("request-1"),
		Destinations: []types.OutputDestination{{Id: aws.String("archive"), Settings: settings}},
		EncoderSettings: &types.EncoderSettings{
			TimecodeConfig:    &types.TimecodeConfig{Source: types.TimecodeConfigSourceSystemclock},
			VideoDescriptions: []types.VideoDescription{{Name: aws.String("video")}},
			AudioDescriptions: []types.AudioDescription{{Name: aws.String("audio"), AudioSelectorName: aws.String("default")}},
			OutputGroups: []types.OutputGroup{{
				Name: aws.String("archive"),
				OutputGroupSettings: &types.OutputGroupSettings{ArchiveGroupSettings: &types.ArchiveGroupSettings{
					Destination: &types.OutputLocationRef{DestinationRefId: aws.String("archive")},
				}},
				Outputs: []types.Output{{
					VideoDescriptionName:  aws.String("video"),
					AudioDescriptionNames: []string{"audio"},
					OutputSettings: &types.OutputSettings{ArchiveOutputSettings: &types.ArchiveOutputSettings{
						ContainerSettings: &types.ArchiveContainerSettings{M2tsSettings: &types.M2tsSettings{}},
					}},
				}},
			}},
		},
	}
}

// singlePipeline returns a valid request for a SINGLE_PIPELINE channel.
func singlePipeline() *medialive.CreateChannelInput {
	return channelInput(types.ChannelClassSinglePipeline, "s3ssl://app-media/a")
}

// standard returns a valid request for a STANDARD channel.
func standard() *medialive.CreateChannelInput {
	return channelInput(types.ChannelClassStandard, "s3ssl://app-media/a", "s3ssl://app-media/b")
}

// createChannel creates a channel from in with f, and returns its ID.
func createChannel(ctx context.Context, f *fakeAPI, in *medialive.CreateChannelInput) (string, error) {
	out, err := f.CreateChannel(ctx, in)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Channel.Id), nil
}

// idleChannel creates a channel from in with f, and reads it until it is IDLE.
func idleChannel(ctx context.Context, f *fakeAPI, in *medialive.CreateChannelInput) (string, error) {
	id, err := createChannel(ctx, f, in)
	if err != nil {
		return "", err
	}
	for range 3 {
		out, err := f.DescribeChannel(ctx, &medialive.DescribeChannelInput{ChannelId: aws.String(id)})
		if err != nil {
			return "", err
		}
		if out.State == types.ChannelStateIdle {
			return id, nil
		}
	}
	return "", fmt.Errorf("channel %s isn't IDLE", id)
}

// runningChannel creates a channel with f and starts it.
func runningChannel(ctx context.Context, f *fakeAPI) (string, error) {
	id, err := idleChannel(ctx, f, singlePipeline())
	if err != nil {
		return "", err
	}
	f.state[id] = types.ChannelStateRunning
	return id, nil
}

// createRule returns a rule that CreateChannel rejects a request newInput returns, changed by change, with
// UnprocessableEntityException.
func createRule(rule string, newInput func() *medialive.CreateChannelInput, change func(in *medialive.CreateChannelInput)) nativetest.Rule {
	return nativetest.Rule{
		Rule: rule,
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			in := newInput()
			change(in)
			_, err := newFakeAPI(store).CreateChannel(ctx, in)
			return err
		},
		Code: "UnprocessableEntityException",
	}
}

// updateChannel creates an IDLE SINGLE_PIPELINE channel and updates it with in, whose ChannelId the call sets.
func updateChannel(ctx context.Context, store *awsfake.CloudControl, in *medialive.UpdateChannelInput) error {
	f := newFakeAPI(store)
	id, err := idleChannel(ctx, f, singlePipeline())
	if err != nil {
		return err
	}
	in.ChannelId = aws.String(id)
	_, err = f.UpdateChannel(ctx, in)
	return err
}

// whileRunning returns a rule that call changes a running channel with ConflictException.
func whileRunning(doc string, call func(ctx context.Context, f *fakeAPI, id string) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "the channel is idle, not running",
		Doc:  doc,
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			id, err := runningChannel(ctx, f)
			if err != nil {
				return err
			}
			return call(ctx, f, id)
		},
		Code: "ConflictException",
	}
}

// missing returns a rule that call fails with NotFoundException for a channel that doesn't exist.
func missing(call func(ctx context.Context, f *fakeAPI, id string) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "the channel exists",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			return call(ctx, newFakeAPI(store), "9999999")
		},
		Code: "NotFoundException",
	}
}

// whileRunningTags returns a rule that call changes the tags of a running channel.
func whileRunningTags(call func(ctx context.Context, f *fakeAPI, arn string) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "tags change while the channel is running or idle",
		Doc:  editingPage,
		Check: func(ctx context.Context, store *awsfake.CloudControl) error {
			f := newFakeAPI(store)
			id, err := runningChannel(ctx, f)
			if err != nil {
				return err
			}
			return call(ctx, f, arnPrefix+id)
		},
	}
}

// createTags creates a channel and adds tags to it.
func createTags(ctx context.Context, store *awsfake.CloudControl, tags map[string]string) error {
	f := newFakeAPI(store)
	id, err := idleChannel(ctx, f, singlePipeline())
	if err != nil {
		return err
	}
	_, err = f.CreateTags(ctx, &medialive.CreateTagsInput{ResourceArn: aws.String(arnPrefix + id), Tags: tags})
	return err
}

// tagRule returns a rule that CreateTags rejects tags with BadRequestException.
func tagRule(rule string, tags map[string]string) nativetest.Rule {
	return nativetest.Rule{
		Rule: rule,
		Doc:  tagRules,
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			return createTags(ctx, store, tags)
		},
		Code: "BadRequestException",
	}
}

// channelAPI records the MediaLive calls Channel makes.
var channelAPI = map[string]nativetest.Call{
	"CreateChannel": {
		Doc: channelsPage,
		Rules: []nativetest.Rule{
			{
				Rule: "a create sent again with the same RequestId returns the first channel and creates no other",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					first, err := createChannel(ctx, f, singlePipeline())
					if err != nil {
						return err
					}
					again, err := createChannel(ctx, f, singlePipeline())
					if err != nil {
						return err
					}
					if first != again || len(store.List(channelType)) != 1 {
						return fmt.Errorf("the resend created channel %s after %s", again, first)
					}
					return nil
				},
			},
			createRule("a SINGLE_PIPELINE channel has one pipeline, so each destination's Settings has one address", singlePipeline, func(in *medialive.CreateChannelInput) {
				in.Destinations[0].Settings = append(in.Destinations[0].Settings, types.OutputDestinationSettings{Url: aws.String("s3ssl://app-media/b")})
			}),
			createRule("a STANDARD channel has two pipelines, so each destination's Settings has two addresses", standard, func(in *medialive.CreateChannelInput) {
				in.Destinations[0].Settings = in.Destinations[0].Settings[:1]
			}),
			createRule("an output group's destination is a destination of the channel", singlePipeline, func(in *medialive.CreateChannelInput) {
				in.EncoderSettings.OutputGroups[0].OutputGroupSettings.ArchiveGroupSettings.Destination.DestinationRefId = aws.String("other")
			}),
			createRule("an output's VideoDescriptionName is a video description of the channel", singlePipeline, func(in *medialive.CreateChannelInput) {
				in.EncoderSettings.OutputGroups[0].Outputs[0].VideoDescriptionName = aws.String("other")
			}),
			createRule("an output's AudioDescriptionNames are audio descriptions of the channel", singlePipeline, func(in *medialive.CreateChannelInput) {
				in.EncoderSettings.OutputGroups[0].Outputs[0].AudioDescriptionNames = []string{"audio", "other"}
			}),
			createRule("an output group's Name is at most 32 letters, numbers, and underscores", singlePipeline, func(in *medialive.CreateChannelInput) {
				in.EncoderSettings.OutputGroups[0].Name = aws.String("app-archive")
			}),
			createRule("an empty DestinationRefId still names a destination the channel must have", singlePipeline, func(in *medialive.CreateChannelInput) {
				in.EncoderSettings.OutputGroups[0].OutputGroupSettings.ArchiveGroupSettings.Destination.DestinationRefId = aws.String("")
			}),
			createRule("an empty VideoDescriptionName still names a video description the channel must have", singlePipeline, func(in *medialive.CreateChannelInput) {
				in.EncoderSettings.OutputGroups[0].Outputs[0].VideoDescriptionName = aws.String("")
			}),
			createRule("an empty name in AudioDescriptionNames still names an audio description the channel must have", singlePipeline, func(in *medialive.CreateChannelInput) {
				in.EncoderSettings.OutputGroups[0].Outputs[0].AudioDescriptionNames = []string{"audio", ""}
			}),
		},
		Unclear: errorsUnclear + "The fake returns UnprocessableEntityException, the 422 ChannelConfigurationValidationError, " +
			"for settings that don't fit together. The page doesn't say how long AWS keeps a RequestId, or what it does " +
			"when one comes again with other settings; the fake returns the first channel. It gives no default for a " +
			"ChannelClass or LogLevel left out; the fake stores none and checks no destination addresses without a class, " +
			"and the handler sends DISABLED for a removed LogLevel. EncoderSettings needs AudioDescriptions, OutputGroups, " +
			"TimecodeConfig, and VideoDescriptions, but the SDK checks that before it sends the call, so the fake doesn't. " +
			"MediaPackageSettings has \"one destination for both encoders\", but a MediaPackage v2 destination names " +
			"endpoint 1 or 2, so the fake doesn't count them. The tag restrictions apply to Tags too, but the page doesn't " +
			"say whether a broken one gets 400 or 422; the fake checks them only in CreateTags.",
	},
	"DescribeChannel": {
		Doc: channelPage,
		Rules: []nativetest.Rule{
			missing(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.DescribeChannel(ctx, &medialive.DescribeChannelInput{ChannelId: aws.String(id)})
				return err
			}),
		},
		Unclear: "The page lists DELETED as a channel state but doesn't say how long AWS returns a deleted channel. " +
			"The fake returns it as DELETED for one read, then NotFoundException.",
	},
	"UpdateChannel": {
		Doc: channelPage,
		Rules: []nativetest.Rule{
			whileRunning(editingPage, func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.UpdateChannel(ctx, &medialive.UpdateChannelInput{ChannelId: aws.String(id), LogLevel: types.LogLevelDebug})
				return err
			}),
			{
				Rule: "an update that leaves out RoleArn removes the channel's role",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := idleChannel(ctx, f, singlePipeline())
					if err != nil {
						return err
					}
					if _, err := f.UpdateChannel(ctx, &medialive.UpdateChannelInput{ChannelId: aws.String(id), LogLevel: types.LogLevelDebug}); err != nil {
						return err
					}
					if role, ok := store.Resource(channelType, id)["RoleArn"]; ok {
						return fmt.Errorf("the channel kept role %v", role)
					}
					return nil
				},
			},
			{
				Rule: "each destination's Settings has one address for each pipeline of the channel's class",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return updateChannel(ctx, store, &medialive.UpdateChannelInput{RoleArn: aws.String(testRoleArn), Destinations: standard().Destinations})
				},
				Code: "UnprocessableEntityException",
			},
			{
				Rule: "the channel's output groups name destinations of the channel",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					destinations := singlePipeline().Destinations
					destinations[0].Id = aws.String("other")
					return updateChannel(ctx, store, &medialive.UpdateChannelInput{RoleArn: aws.String(testRoleArn), Destinations: destinations})
				},
				Code: "UnprocessableEntityException",
			},
		},
		Unclear: errorsUnclear + "The fake returns ConflictException, the 409 ResourceConflict, for a channel that isn't idle, " +
			"and UnprocessableEntityException for settings that don't fit together. The page says an update without " +
			"roleArn removes the role, but the recorded replay (testdata/channel.json) shows AWS kept the role after an " +
			"UpdateChannel without it. The fake removes it, as the page says, and the handler sends the template's role on " +
			"every UpdateChannel, which is right either way.",
	},
	"UpdateChannelClass": {
		Doc: classPage,
		Rules: []nativetest.Rule{
			whileRunning(classChange, func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.UpdateChannelClass(ctx, &medialive.UpdateChannelClassInput{
					ChannelId: aws.String(id), ChannelClass: types.ChannelClassStandard, Destinations: standard().Destinations,
				})
				return err
			}),
			{
				Rule: "a change to STANDARD needs a second address in each destination",
				Doc:  classChange,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := idleChannel(ctx, f, singlePipeline())
					if err != nil {
						return err
					}
					_, err = f.UpdateChannelClass(ctx, &medialive.UpdateChannelClassInput{ChannelId: aws.String(id), ChannelClass: types.ChannelClassStandard})
					return err
				},
				Code: "UnprocessableEntityException",
			},
			{
				Rule: "a change to SINGLE_PIPELINE removes the second address of each destination",
				Doc:  classChange,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					id, err := idleChannel(ctx, f, standard())
					if err != nil {
						return err
					}
					if _, err := f.UpdateChannelClass(ctx, &medialive.UpdateChannelClassInput{ChannelId: aws.String(id), ChannelClass: types.ChannelClassSinglePipeline}); err != nil {
						return err
					}
					out, err := f.DescribeChannel(ctx, &medialive.DescribeChannelInput{ChannelId: aws.String(id)})
					if err != nil {
						return err
					}
					if n := len(out.Destinations[0].Settings); n != 1 || out.ChannelClass != types.ChannelClassSinglePipeline {
						return fmt.Errorf("the %s channel has %d addresses", out.ChannelClass, n)
					}
					return nil
				},
			},
		},
		Unclear: errorsUnclear + "The fake returns ConflictException, the 409 ResourceConflict, for a channel that isn't idle, " +
			"and UnprocessableEntityException for destinations that don't fit the class. The docs don't say what AWS " +
			"does for a change to the class the channel has, which the fake accepts, or for a change to SINGLE_PIPELINE " +
			"with destinations that have two addresses, whose second address the fake removes too.",
	},
	"DeleteChannel": {
		Doc: channelPage,
		Rules: []nativetest.Rule{
			whileRunning(editingPage, func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.DeleteChannel(ctx, &medialive.DeleteChannelInput{ChannelId: aws.String(id)})
				return err
			}),
			missing(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.DeleteChannel(ctx, &medialive.DeleteChannelInput{ChannelId: aws.String(id)})
				return err
			}),
		},
		Unclear: errorsUnclear + "The fake returns ConflictException, the 409 ResourceConflict, for a channel that isn't idle, " +
			"including one AWS is already deleting, which the handler then reads and treats as deleted.",
	},
	"ListChannels": {
		Doc: channelsPage,
		Unclear: "The page lists maxResults and nextToken without limits or a page size. The handler sends no maxResults " +
			"and follows nextToken; the fake returns every channel on one page unless a test sets its page size.",
	},
	"CreateTags": {
		Doc: tagsPage,
		Rules: []nativetest.Rule{
			missing(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.CreateTags(ctx, &medialive.CreateTagsInput{ResourceArn: aws.String(arnPrefix + id), Tags: map[string]string{"team": "a"}})
				return err
			}),
			whileRunningTags(func(ctx context.Context, f *fakeAPI, arn string) error {
				_, err := f.CreateTags(ctx, &medialive.CreateTagsInput{ResourceArn: aws.String(arn), Tags: map[string]string{"team": "a"}})
				return err
			}),
			tagRule("keys don't start with aws:, which is reserved for AWS", map[string]string{"aws:team": "a"}),
			tagRule("a key is at most 128 characters", map[string]string{strings.Repeat("k", 129): "a"}),
			tagRule("a value is at most 256 characters", map[string]string{"team": strings.Repeat("v", 257)}),
			tagRule("keys and values have only letters, numbers, spaces, and _ . : / = + - @", map[string]string{"team": "a#b"}),
			tagRule("a channel has at most 50 tags", func() map[string]string {
				tags := map[string]string{}
				for i := range 51 {
					tags["tag"+strconv.Itoa(i)] = "a"
				}
				return tags
			}()),
		},
		Unclear: "The tagging page doesn't say which error a broken tag restriction gets. The fake returns " +
			"BadRequestException, the tags page's 400 InvalidRequest, its only response for a bad request.",
	},
	"DeleteTags": {
		Doc: tagsPage,
		Rules: []nativetest.Rule{
			missing(func(ctx context.Context, f *fakeAPI, id string) error {
				_, err := f.DeleteTags(ctx, &medialive.DeleteTagsInput{ResourceArn: aws.String(arnPrefix + id), TagKeys: []string{"team"}})
				return err
			}),
			whileRunningTags(func(ctx context.Context, f *fakeAPI, arn string) error {
				_, err := f.DeleteTags(ctx, &medialive.DeleteTagsInput{ResourceArn: aws.String(arn), TagKeys: []string{"team"}})
				return err
			}),
		},
		Unclear: "The page doesn't say what AWS does for a key the channel doesn't have. The fake ignores it.",
	},
}
