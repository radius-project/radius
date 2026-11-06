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
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const arnPrefix = "arn:aws:sagemaker:us-west-2:123456789012:experiment-trial-component/"

// fakeAPI is an in-memory SageMaker API for trial components. It keeps them in an awsfake store, in the type's
// property format, so the suite's Cloud Control sees the same resources. A deleted trial component is still described
// once before it is gone.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu       sync.Mutex
	deleting map[string]bool

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, deleting: map[string]bool{}, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// change makes a call that changes AWS, failing it as the failer says.
func (f *fakeAPI) change(apply func() error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

func (f *fakeAPI) CreateTrialComponent(ctx context.Context, in *sagemaker.CreateTrialComponentInput, _ ...func(*sagemaker.Options)) (*sagemaker.CreateTrialComponentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["CreateTrialComponent"]; err != nil {
		return nil, err
	}
	name := aws.ToString(in.TrialComponentName)
	return &sagemaker.CreateTrialComponentOutput{TrialComponentArn: aws.String(arnPrefix + name)}, f.change(func() error {
		if f.store.Resource(trialComponentType, name) != nil {
			return fault("ResourceInUse")
		}
		props, err := awsapi.EncodeObject(in)
		if err != nil {
			return err
		}
		f.store.Put(trialComponentType, name, props)
		return nil
	})
}

func (f *fakeAPI) DescribeTrialComponent(ctx context.Context, in *sagemaker.DescribeTrialComponentInput, _ ...func(*sagemaker.Options)) (*sagemaker.DescribeTrialComponentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DescribeTrialComponent"]; err != nil {
		return nil, err
	}
	name := aws.ToString(in.TrialComponentName)
	props := f.store.Resource(trialComponentType, name)
	if props == nil {
		return nil, fault("ResourceNotFound")
	}
	if f.deleting[name] {
		delete(f.deleting, name)
		f.store.Remove(trialComponentType, name)
	}
	out := &sagemaker.DescribeTrialComponentOutput{}
	if err := awsapi.Decode(awsapi.Without(props, "Arn", "Id", "Tags"), out); err != nil {
		return nil, err
	}
	created := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	out.TrialComponentName = aws.String(name)
	out.TrialComponentArn = aws.String(arnPrefix + name)
	out.CreationTime, out.LastModifiedTime = &created, &created
	return out, nil
}

func (f *fakeAPI) UpdateTrialComponent(ctx context.Context, in *sagemaker.UpdateTrialComponentInput, _ ...func(*sagemaker.Options)) (*sagemaker.UpdateTrialComponentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := aws.ToString(in.TrialComponentName)
	return &sagemaker.UpdateTrialComponentOutput{}, f.change(func() error {
		props := f.store.Resource(trialComponentType, name)
		if props == nil {
			return fault("ResourceNotFound")
		}
		values, err := awsapi.EncodeObject(in)
		if err != nil {
			return err
		}
		delete(values, "TrialComponentName")
		for k, v := range values {
			props[k] = v
		}
		f.store.Put(trialComponentType, name, props)
		return nil
	})
}

func (f *fakeAPI) DeleteTrialComponent(ctx context.Context, in *sagemaker.DeleteTrialComponentInput, _ ...func(*sagemaker.Options)) (*sagemaker.DeleteTrialComponentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := aws.ToString(in.TrialComponentName)
	return &sagemaker.DeleteTrialComponentOutput{}, f.change(func() error {
		if f.store.Resource(trialComponentType, name) == nil || f.deleting[name] {
			return fault("ResourceNotFound")
		}
		f.deleting[name] = true
		return nil
	})
}

// ListTrialComponents returns one trial component per page.
func (f *fakeAPI) ListTrialComponents(ctx context.Context, in *sagemaker.ListTrialComponentsInput, _ ...func(*sagemaker.Options)) (*sagemaker.ListTrialComponentsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ListTrialComponents"]; err != nil {
		return nil, err
	}
	names := f.store.List(trialComponentType)
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	out := &sagemaker.ListTrialComponentsOutput{}
	if start < len(names) {
		out.TrialComponentSummaries = []types.TrialComponentSummary{{TrialComponentName: aws.String(names[start])}}
	}
	if start+1 < len(names) {
		out.NextToken = aws.String(strconv.Itoa(start + 1))
	}
	return out, nil
}

func nameOf(arn *string) string {
	return strings.TrimPrefix(aws.ToString(arn), arnPrefix)
}

// tagsOf returns the stored tags of the trial component arn names.
func (f *fakeAPI) tagsOf(arn *string) (map[string]any, map[string]string, error) {
	props := f.store.Resource(trialComponentType, nameOf(arn))
	if props == nil {
		return nil, nil, fault("ResourceNotFound")
	}
	tags := map[string]string{}
	for _, t := range awsapi.Tags(props) {
		tags[t.Key] = t.Value
	}
	return props, tags, nil
}

func (f *fakeAPI) putTags(arn *string, props map[string]any, tags map[string]string) {
	if list := awsapi.TagList(tags); list != nil {
		props["Tags"] = list
	} else {
		delete(props, "Tags")
	}
	f.store.Put(trialComponentType, nameOf(arn), props)
}

func (f *fakeAPI) AddTags(ctx context.Context, in *sagemaker.AddTagsInput, _ ...func(*sagemaker.Options)) (*sagemaker.AddTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &sagemaker.AddTagsOutput{}, f.change(func() error {
		props, tags, err := f.tagsOf(in.ResourceArn)
		if err != nil {
			return err
		}
		for _, t := range in.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		f.putTags(in.ResourceArn, props, tags)
		return nil
	})
}

func (f *fakeAPI) DeleteTags(ctx context.Context, in *sagemaker.DeleteTagsInput, _ ...func(*sagemaker.Options)) (*sagemaker.DeleteTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &sagemaker.DeleteTagsOutput{}, f.change(func() error {
		props, tags, err := f.tagsOf(in.ResourceArn)
		if err != nil {
			return err
		}
		for key := range tags {
			if slices.Contains(in.TagKeys, key) {
				delete(tags, key)
			}
		}
		f.putTags(in.ResourceArn, props, tags)
		return nil
	})
}

// ListTags returns one tag per page.
func (f *fakeAPI) ListTags(ctx context.Context, in *sagemaker.ListTagsInput, _ ...func(*sagemaker.Options)) (*sagemaker.ListTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ListTags"]; err != nil {
		return nil, err
	}
	_, tags, err := f.tagsOf(in.ResourceArn)
	if err != nil {
		return nil, err
	}
	list := awsapi.Tags(map[string]any{"Tags": awsapi.TagList(tags)})
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	out := &sagemaker.ListTagsOutput{}
	if start < len(list) {
		out.Tags = []types.Tag{{Key: aws.String(list[start].Key), Value: aws.String(list[start].Value)}}
	}
	if start+1 < len(list) {
		out.NextToken = aws.String(strconv.Itoa(start + 1))
	}
	return out, nil
}
