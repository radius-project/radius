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

package sagemaker

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference = "https://docs.aws.amazon.com/sagemaker/latest/APIReference/"
	commonErrors = apiReference + "CommonErrors.html"
	tagDoc       = apiReference + "API_Tag.html"
)

// ruleArn is the ARN of the trial component the rules create.
const ruleArn = arnPrefix + "rule-trial"

// create creates the trial component rule-trial with a fake SageMaker API backed by store, after edit changes the
// input, when edit is set.
func create(ctx context.Context, store *awsfake.CloudControl, edit func(in *sagemaker.CreateTrialComponentInput)) error {
	in := &sagemaker.CreateTrialComponentInput{TrialComponentName: aws.String("rule-trial")}
	if edit != nil {
		edit(in)
	}
	_, err := newFakeAPI(store).CreateTrialComponent(ctx, in)
	return err
}

// update creates rule-trial, then updates it after edit changes the input, with a fake SageMaker API backed by store.
func update(ctx context.Context, store *awsfake.CloudControl, edit func(in *sagemaker.UpdateTrialComponentInput)) error {
	if err := create(ctx, store, nil); err != nil {
		return err
	}
	in := &sagemaker.UpdateTrialComponentInput{TrialComponentName: aws.String("rule-trial")}
	edit(in)
	_, err := newFakeAPI(store).UpdateTrialComponent(ctx, in)
	return err
}

// tagCall creates rule-trial, then calls call with a fake SageMaker API backed by store.
func tagCall(ctx context.Context, store *awsfake.CloudControl, call func(f *fakeAPI) error) error {
	if err := create(ctx, store, nil); err != nil {
		return err
	}
	return call(newFakeAPI(store))
}

// validationRule returns a rule that edit, applied to an input of call, breaks with ValidationError.
func validationRule[T any](rule string, doc string, call func(ctx context.Context, store *awsfake.CloudControl, edit func(in *T)) error, edit func(in *T)) nativetest.Rule {
	return nativetest.Rule{
		Rule: rule,
		Doc:  doc,
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			return call(ctx, store, edit)
		},
		Code: "ValidationError",
	}
}

// many returns n tags with distinct keys.
func many(n int) []types.Tag {
	out := []types.Tag{}
	for i := range n {
		out = append(out, types.Tag{Key: aws.String(fmt.Sprintf("k%d", i)), Value: aws.String("v")})
	}
	return out
}

// artifacts returns n artifacts with distinct keys.
func artifacts(n int) map[string]types.TrialComponentArtifact {
	out := map[string]types.TrialComponentArtifact{}
	for i := range n {
		out[fmt.Sprintf("a%d", i)] = types.TrialComponentArtifact{Value: aws.String("s3://bucket/key")}
	}
	return out
}

// valueRules returns the rules SageMaker's API reference gives for the values CreateTrialComponent and
// UpdateTrialComponent both take, with call making the call after an edit sets them on its input.
func valueRules[T any](call func(ctx context.Context, store *awsfake.CloudControl, edit func(in *T)) error, set func(in *T, v trialComponentValues)) []nativetest.Rule {
	with := func(v trialComponentValues) func(in *T) {
		return func(in *T) { set(in, v) }
	}
	return []nativetest.Rule{
		validationRule("TrialComponentName is 1 to 120 characters of [a-zA-Z0-9](-*[a-zA-Z0-9]){0,119}", commonErrors, call,
			with(trialComponentValues{name: aws.String("rule_trial")})),
		validationRule("DisplayName is 1 to 120 characters of [a-zA-Z0-9](-*[a-zA-Z0-9]){0,119}", commonErrors, call,
			with(trialComponentValues{displayName: aws.String("Rule trial")})),
		validationRule("Status.PrimaryStatus is InProgress, Completed, Failed, Stopping, or Stopped", apiReference+"API_TrialComponentStatus.html", call,
			with(trialComponentValues{status: &types.TrialComponentStatus{PrimaryStatus: "Running"}})),
		validationRule("Status.Message is at most 1024 characters", apiReference+"API_TrialComponentStatus.html", call,
			with(trialComponentValues{status: &types.TrialComponentStatus{Message: aws.String(strings.Repeat("m", 1025))}})),
		validationRule("Parameters has at most 300 entries, with keys of at most 320 characters", commonErrors, call,
			with(trialComponentValues{parameters: map[string]types.TrialComponentParameterValue{
				strings.Repeat("p", 321): &types.TrialComponentParameterValueMemberNumberValue{Value: 1},
			}})),
		validationRule("a parameter's StringValue is at most 2500 characters", apiReference+"API_TrialComponentParameterValue.html", call,
			with(trialComponentValues{parameters: map[string]types.TrialComponentParameterValue{
				"p": &types.TrialComponentParameterValueMemberStringValue{Value: strings.Repeat("s", 2501)},
			}})),
		validationRule("InputArtifacts has at most 60 entries, with keys of at most 128 characters", commonErrors, call,
			with(trialComponentValues{inputArtifacts: artifacts(61)})),
		validationRule("OutputArtifacts has at most 60 entries, with keys of at most 128 characters", commonErrors, call,
			with(trialComponentValues{outputArtifacts: map[string]types.TrialComponentArtifact{
				strings.Repeat("a", 129): {Value: aws.String("s3://bucket/key")},
			}})),
		validationRule("an artifact has a Value of at most 2048 characters, and a MediaType of at most 64 characters of [-\\w]+/[-\\w+]+", apiReference+"API_TrialComponentArtifact.html", call,
			with(trialComponentValues{inputArtifacts: map[string]types.TrialComponentArtifact{
				"a": {Value: aws.String("s3://bucket/key"), MediaType: aws.String("csv")},
			}})),
	}
}

// setCreate sets the values v sets on a CreateTrialComponentInput.
func setCreate(in *sagemaker.CreateTrialComponentInput, v trialComponentValues) {
	if v.name != nil {
		in.TrialComponentName = v.name
	}
	in.DisplayName, in.Status, in.Parameters, in.InputArtifacts, in.OutputArtifacts = v.displayName, v.status, v.parameters, v.inputArtifacts, v.outputArtifacts
}

// setUpdate sets the values v sets on an UpdateTrialComponentInput.
func setUpdate(in *sagemaker.UpdateTrialComponentInput, v trialComponentValues) {
	if v.name != nil {
		in.TrialComponentName = v.name
	}
	in.DisplayName, in.Status, in.Parameters, in.InputArtifacts, in.OutputArtifacts = v.displayName, v.status, v.parameters, v.inputArtifacts, v.outputArtifacts
}

// wideKey and wideValue are a tag key and value at their limits of 128 and 256 characters, of a letter that takes
// three bytes in UTF-8, so a fake that counts bytes rejects them.
var (
	wideKey   = strings.Repeat("界", 128)
	wideValue = strings.Repeat("界", 256)
)

// tagRules returns the rules SageMaker's API reference gives for the tags of a call, made with call.
func tagRules(call func(ctx context.Context, f *fakeAPI, tags []types.Tag) error) []nativetest.Rule {
	rule := func(text string, tags []types.Tag) nativetest.Rule {
		return nativetest.Rule{
			Rule: text,
			Doc:  tagDoc,
			Break: func(ctx context.Context, store *awsfake.CloudControl) error {
				return tagCall(ctx, store, func(f *fakeAPI) error { return call(ctx, f, tags) })
			},
			Code: "ValidationError",
		}
	}
	return []nativetest.Rule{
		rule("a call takes at most 50 tags", many(51)),
		rule("a tag key is 1 to 128 characters", []types.Tag{{Key: aws.String(""), Value: aws.String("v")}}),
		rule("a tag value is at most 256 characters", []types.Tag{{Key: aws.String("k"), Value: aws.String(strings.Repeat("v", 257))}}),
		rule("tag keys and values match ([\\p{L}\\p{Z}\\p{N}_.:/=+\\-@]*)", []types.Tag{{Key: aws.String("k"), Value: aws.String("a*b")}}),
		rule("a tag has a value", []types.Tag{{Key: aws.String("k")}}),
		{
			Rule: "tag keys and values may have letters of any language, and their limits count characters, not bytes",
			Doc:  tagDoc,
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				return tagCall(ctx, store, func(f *fakeAPI) error {
					return call(ctx, f, []types.Tag{{Key: aws.String(wideKey), Value: aws.String(wideValue)}})
				})
			},
		},
	}
}

// resourceArnRule returns the rule SageMaker's API reference gives for the ResourceArn of a tag call, made with call.
func resourceArnRule(call func(ctx context.Context, f *fakeAPI, arn string) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "ResourceArn is a SageMaker ARN of at most 256 characters",
		Doc:  commonErrors,
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			return tagCall(ctx, store, func(f *fakeAPI) error { return call(ctx, f, "arn:aws:s3:::rule-trial") })
		},
		Code: "ValidationError",
	}
}

// tagUnclear is what AWS's docs leave unclear about every tag call.
const tagUnclear = "The page lists no error for a resource that doesn't exist; the fake returns ResourceNotFound, and the " +
	"handler treats it as not found."

// trialComponentAPI records the SageMaker calls TrialComponent makes.
var trialComponentAPI = map[string]nativetest.Call{
	"AddTags": {
		Doc: apiReference + "API_AddTags.html",
		Rules: append(tagRules(func(ctx context.Context, f *fakeAPI, tags []types.Tag) error {
			_, err := f.AddTags(ctx, &sagemaker.AddTagsInput{ResourceArn: aws.String(ruleArn), Tags: tags})
			return err
		}), resourceArnRule(func(ctx context.Context, f *fakeAPI, arn string) error {
			_, err := f.AddTags(ctx, &sagemaker.AddTagsInput{ResourceArn: aws.String(arn), Tags: many(1)})
			return err
		}), nativetest.Rule{
			Rule: "AddTags overwrites the value of a tag key the resource has",
			Check: func(ctx context.Context, store *awsfake.CloudControl) error {
				return tagCall(ctx, store, func(f *fakeAPI) error {
					for _, value := range []string{"a", "b"} {
						if _, err := f.AddTags(ctx, &sagemaker.AddTagsInput{
							ResourceArn: aws.String(ruleArn),
							Tags:        []types.Tag{{Key: aws.String("team"), Value: aws.String(value)}},
						}); err != nil {
							return err
						}
					}
					out, err := f.ListTags(ctx, &sagemaker.ListTagsInput{ResourceArn: aws.String(ruleArn)})
					if err != nil {
						return err
					}
					if len(out.Tags) != 1 || aws.ToString(out.Tags[0].Value) != "b" {
						return fmt.Errorf("the resource has tags %v", out.Tags)
					}
					return nil
				})
			},
		}),
		Unclear: tagUnclear + " The page doesn't say what SageMaker does with two tags of the same key in one call; " +
			"the fake keeps the last.",
	},
	"CreateTrialComponent": {
		Doc: apiReference + "API_CreateTrialComponent.html",
		Rules: append(valueRules(create, setCreate),
			validationRule("Tags has at most 50 tags", commonErrors, create, func(in *sagemaker.CreateTrialComponentInput) { in.Tags = many(51) }),
			nativetest.Rule{
				Rule: "tag keys and values may have letters of any language, and their limits count characters, not bytes",
				Doc:  tagDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					return create(ctx, store, func(in *sagemaker.CreateTrialComponentInput) {
						in.Tags = []types.Tag{{Key: aws.String(wideKey), Value: aws.String(wideValue)}}
					})
				},
			},
			validationRule("each MetadataProperties value is at most 1024 characters", apiReference+"API_MetadataProperties.html", create, func(in *sagemaker.CreateTrialComponentInput) {
				in.MetadataProperties = &types.MetadataProperties{CommitId: aws.String(strings.Repeat("c", 1025))}
			}),
			nativetest.Rule{
				Rule: "the name is unique in the account and isn't case-sensitive, so RULE-TRIAL can't be created next to rule-trial",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := create(ctx, store, nil); err != nil {
						return err
					}
					if err := create(ctx, store, func(in *sagemaker.CreateTrialComponentInput) { in.TrialComponentName = aws.String("RULE-TRIAL") }); err == nil {
						return fmt.Errorf("the fake created RULE-TRIAL next to rule-trial")
					}
					return nil
				},
			},
		),
		Unclear: "The page doesn't name the error for a name already in use; the fake returns ResourceInUse. It lists " +
			"ResourceLimitExceeded without the limit. It lists InProgress, Completed, and Failed as states, while " +
			"TrialComponentStatus also allows Stopping and Stopped; the fake takes all five. It says the console shows " +
			"TrialComponentName when DisplayName isn't set, but not whether DescribeTrialComponent returns it; the fake " +
			"returns no DisplayName. It states no rule for StartTime and EndTime. The Go SDK can't send both NumberValue " +
			"and StringValue for a parameter, so that rule isn't checked. The type has no hyperparameters or artifacts, " +
			"so the fake checks their limits but doesn't keep them.",
	},
	"DeleteTags": {
		Doc: apiReference + "API_DeleteTags.html",
		Rules: []nativetest.Rule{
			{
				Rule: "tag keys may have letters of any language, and their limit counts characters, not bytes",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					return tagCall(ctx, store, func(f *fakeAPI) error {
						_, err := f.DeleteTags(ctx, &sagemaker.DeleteTagsInput{ResourceArn: aws.String(ruleArn), TagKeys: []string{wideKey}})
						return err
					})
				},
			},
			resourceArnRule(func(ctx context.Context, f *fakeAPI, arn string) error {
				_, err := f.DeleteTags(ctx, &sagemaker.DeleteTagsInput{ResourceArn: aws.String(arn), TagKeys: []string{"k"}})
				return err
			}),
			{
				Rule: "TagKeys has 1 to 50 keys",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return tagCall(ctx, store, func(f *fakeAPI) error {
						_, err := f.DeleteTags(ctx, &sagemaker.DeleteTagsInput{ResourceArn: aws.String(ruleArn), TagKeys: []string{}})
						return err
					})
				},
				Code: "ValidationError",
			},
			{
				Rule: "a tag key is 1 to 128 characters of ([\\p{L}\\p{Z}\\p{N}_.:/=+\\-@]*)",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return tagCall(ctx, store, func(f *fakeAPI) error {
						_, err := f.DeleteTags(ctx, &sagemaker.DeleteTagsInput{ResourceArn: aws.String(ruleArn), TagKeys: []string{strings.Repeat("k", 129)}})
						return err
					})
				},
				Code: "ValidationError",
			},
		},
		Unclear: tagUnclear + " It doesn't say what SageMaker does for a key the resource doesn't have; the fake " +
			"accepts it and changes nothing.",
	},
	"DeleteTrialComponent": {
		Doc: apiReference + "API_DeleteTrialComponent.html",
		Rules: []nativetest.Rule{
			{
				Rule: "TrialComponentName is 1 to 120 characters of [a-zA-Z0-9](-*[a-zA-Z0-9]){0,119}",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeleteTrialComponent(ctx, &sagemaker.DeleteTrialComponentInput{TrialComponentName: aws.String("-rule")})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "the trial component exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeleteTrialComponent(ctx, &sagemaker.DeleteTrialComponentInput{TrialComponentName: aws.String("rule-trial")})
					return err
				},
				Code: "ResourceNotFound",
			},
		},
		Unclear: "The page says a trial component must be disassociated from every trial before it is deleted, but " +
			"doesn't name the error; the fake has no trials. It doesn't say whether the trial component is gone at " +
			"once; the fake describes it once more after the delete.",
	},
	"DescribeTrialComponent": {
		Doc: apiReference + "API_DescribeTrialComponent.html",
		Rules: []nativetest.Rule{
			{
				Rule: "TrialComponentName is a name or the ARN of one, of 1 to 256 characters",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DescribeTrialComponent(ctx, &sagemaker.DescribeTrialComponentInput{TrialComponentName: aws.String("rule trial")})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "TrialComponentName may be the trial component's ARN",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := create(ctx, store, nil); err != nil {
						return err
					}
					_, err := newFakeAPI(store).DescribeTrialComponent(ctx, &sagemaker.DescribeTrialComponentInput{TrialComponentName: aws.String(ruleArn)})
					return err
				},
			},
			{
				Rule: "the trial component exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DescribeTrialComponent(ctx, &sagemaker.DescribeTrialComponentInput{TrialComponentName: aws.String("rule-trial")})
					return err
				},
				Code: "ResourceNotFound",
			},
		},
	},
	"ListTags": {
		Doc: apiReference + "API_ListTags.html",
		Rules: []nativetest.Rule{
			resourceArnRule(func(ctx context.Context, f *fakeAPI, arn string) error {
				_, err := f.ListTags(ctx, &sagemaker.ListTagsInput{ResourceArn: aws.String(arn)})
				return err
			}),
			{
				Rule: "MaxResults is at least 50",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					return tagCall(ctx, store, func(f *fakeAPI) error {
						_, err := f.ListTags(ctx, &sagemaker.ListTagsInput{ResourceArn: aws.String(ruleArn), MaxResults: aws.Int32(49)})
						return err
					})
				},
				Code: "ValidationError",
			},
		},
		Unclear: tagUnclear,
	},
	"ListTrialComponents": {
		Doc: apiReference + "API_ListTrialComponents.html",
		Rules: []nativetest.Rule{
			{
				Rule: "only one of ExperimentName, SourceArn, and TrialName can be set",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).ListTrialComponents(ctx, &sagemaker.ListTrialComponentsInput{ExperimentName: aws.String("e"), TrialName: aws.String("t")})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "MaxResults is 1 to 100",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).ListTrialComponents(ctx, &sagemaker.ListTrialComponentsInput{MaxResults: aws.Int32(101)})
					return err
				},
				Code: "ValidationError",
			},
			{
				Rule: "SortBy is Name or CreationTime, and SortOrder is Ascending or Descending",
				Doc:  commonErrors,
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).ListTrialComponents(ctx, &sagemaker.ListTrialComponentsInput{SortBy: "Status"})
					return err
				},
				Code: "ValidationError",
			},
		},
		Unclear: "The page gives defaults of 10 results, sorted by CreationTime, Descending; the fake returns one per " +
			"page in name order, and the handler reads every page and doesn't rely on the order.",
	},
	"UpdateTrialComponent": {
		Doc: apiReference + "API_UpdateTrialComponent.html",
		Rules: append(valueRules(update, setUpdate),
			validationRule("each key in ParametersToRemove, InputArtifactsToRemove, and OutputArtifactsToRemove is at most 256 characters", commonErrors, update,
				func(in *sagemaker.UpdateTrialComponentInput) {
					in.ParametersToRemove = []string{strings.Repeat("p", 257)}
				}),
			nativetest.Rule{
				Rule: "the trial component exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).UpdateTrialComponent(ctx, &sagemaker.UpdateTrialComponentInput{TrialComponentName: aws.String("rule-trial")})
					return err
				},
				Code: "ResourceNotFound",
			},
		),
		Unclear: "The page lists ConflictException without saying when. It gives no way to clear DisplayName, Status, " +
			"StartTime, or EndTime, so the handler keeps AWS's value when the template removes one.",
	},
}
