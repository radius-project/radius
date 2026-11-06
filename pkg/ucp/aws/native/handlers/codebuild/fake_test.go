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

package codebuild

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	"github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const arnPrefix = "arn:aws:codebuild:us-west-2:123456789012:project/"

// fakeAPI is an in-memory CodeBuild API. It keeps projects in an awsfake store, in the type's property format, so the
// suite's Cloud Control sees the same resources. The project's webhook is its Triggers property. A deleted project
// still shows in one read before it is gone, as AWS's reads can lag.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu       sync.Mutex
	deleting map[string]bool
	calls    []string

	// pageSize, when set, is how many names ListProjects returns in one page.
	pageSize int

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

// call records the call, and fails it when the handler didn't set the region.
func (f *fakeAPI) call(name string, opts []func(*codebuild.Options)) error {
	f.calls = append(f.calls, name)
	o := codebuild.Options{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.Region == "" {
		return errors.New(name + " was called without a region")
	}
	return f.errs[name]
}

// takeCalls returns the calls made, and clears them.
func (f *fakeAPI) takeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.calls
	f.calls = nil
	return out
}

// change runs apply on the stored project, as a call that changes AWS.
func (f *fakeAPI) change(name string, apply func(props map[string]any) error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	props := f.store.Resource(projectType, name)
	if props == nil {
		return fault(notFound)
	}
	if err := apply(props); err != nil {
		return err
	}
	f.store.Put(projectType, name, props)
	return f.failer.After()
}

// set sets the properties the input of a create or update sets, renamed to the type's names. A property the input
// sets to an empty value is removed, as AWS clears it.
func set(props map[string]any, input any, skip ...string) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	values := map[string]any{}
	if err := json.Unmarshal(b, &values); err != nil {
		return err
	}
	for _, name := range skip {
		delete(values, name)
	}
	for name, value := range values {
		if value == nil {
			continue
		}
		encoded, err := awsapi.Encode(value)
		if err != nil {
			return err
		}
		if empty(encoded) {
			delete(props, name)
			continue
		}
		props[name] = encoded
	}
	for _, source := range append(objects(props, "SecondarySources"), object(props, "Source")) {
		if spec, ok := source["Buildspec"]; ok {
			delete(source, "Buildspec")
			source["BuildSpec"] = spec
		}
	}
	return nil
}

func empty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

func (f *fakeAPI) CreateProject(ctx context.Context, in *codebuild.CreateProjectInput, opts ...func(*codebuild.Options)) (*codebuild.CreateProjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateProject", opts); err != nil {
		return nil, err
	}
	name := aws.ToString(in.Name)
	if f.store.Resource(projectType, name) != nil {
		return nil, fault("ResourceAlreadyExistsException")
	}
	if err := f.failer.Before(); err != nil {
		return nil, err
	}
	props := map[string]any{}
	if err := set(props, in); err != nil {
		return nil, err
	}
	f.store.Put(projectType, name, props)
	project, err := f.project(name, props)
	if err != nil {
		return nil, err
	}
	return &codebuild.CreateProjectOutput{Project: &project}, f.failer.After()
}

// project returns the project the stored properties describe, with the defaults AWS fills in.
func (f *fakeAPI) project(name string, props map[string]any) (types.Project, error) {
	p := types.Project{}
	if err := awsapi.Decode(props, &p); err != nil {
		return p, err
	}
	p.Name = aws.String(name)
	p.Arn = aws.String(arnPrefix + name)
	badge, _ := props["BadgeEnabled"].(bool)
	p.Badge = &types.ProjectBadge{BadgeEnabled: badge}
	p.ProjectVisibility = types.ProjectVisibilityTypePrivate
	if v := awsapi.String(props, "Visibility"); v != "" {
		p.ProjectVisibility = types.ProjectVisibilityType(v)
	}
	if triggers, ok := webhookIn(props); ok {
		p.Webhook = &types.Webhook{}
		if err := awsapi.Decode(triggers, p.Webhook); err != nil {
			return p, err
		}
		p.Webhook.Url = aws.String("https://codebuild.us-west-2.amazonaws.com/webhooks/" + name)
	}
	return p, nil
}

// webhookIn returns the Triggers property of the stored project when it turns the webhook on.
func webhookIn(props map[string]any) (map[string]any, bool) {
	triggers, ok := props["Triggers"].(map[string]any)
	return triggers, ok && triggers["Webhook"] == true
}

func (f *fakeAPI) BatchGetProjects(ctx context.Context, in *codebuild.BatchGetProjectsInput, opts ...func(*codebuild.Options)) (*codebuild.BatchGetProjectsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("BatchGetProjects", opts); err != nil {
		return nil, err
	}
	out := &codebuild.BatchGetProjectsOutput{}
	for _, name := range in.Names {
		props := f.store.Resource(projectType, name)
		if props == nil {
			out.ProjectsNotFound = append(out.ProjectsNotFound, name)
			continue
		}
		if f.deleting[name] {
			delete(f.deleting, name)
			f.store.Remove(projectType, name)
		}
		project, err := f.project(name, props)
		if err != nil {
			return nil, err
		}
		out.Projects = append(out.Projects, project)
	}
	return out, nil
}

func (f *fakeAPI) ListProjects(ctx context.Context, in *codebuild.ListProjectsInput, opts ...func(*codebuild.Options)) (*codebuild.ListProjectsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListProjects", opts); err != nil {
		return nil, err
	}
	names := f.store.List(projectType)
	start := 0
	if in.NextToken != nil {
		start, _ = strconv.Atoi(aws.ToString(in.NextToken))
	}
	end := len(names)
	if f.pageSize > 0 && start+f.pageSize < end {
		end = start + f.pageSize
	}
	out := &codebuild.ListProjectsOutput{Projects: names[start:end]}
	if end < len(names) {
		out.NextToken = aws.String(strconv.Itoa(end))
	}
	return out, nil
}

func (f *fakeAPI) UpdateProject(ctx context.Context, in *codebuild.UpdateProjectInput, opts ...func(*codebuild.Options)) (*codebuild.UpdateProjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("UpdateProject", opts); err != nil {
		return nil, err
	}
	return &codebuild.UpdateProjectOutput{}, f.change(aws.ToString(in.Name), func(props map[string]any) error {
		return set(props, in, "Name")
	})
}

func (f *fakeAPI) UpdateProjectVisibility(ctx context.Context, in *codebuild.UpdateProjectVisibilityInput, opts ...func(*codebuild.Options)) (*codebuild.UpdateProjectVisibilityOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("UpdateProjectVisibility", opts); err != nil {
		return nil, err
	}
	name := strings.TrimPrefix(aws.ToString(in.ProjectArn), arnPrefix)
	return &codebuild.UpdateProjectVisibilityOutput{}, f.change(name, func(props map[string]any) error {
		props["Visibility"] = string(in.ProjectVisibility)
		delete(props, "ResourceAccessRole")
		if in.ResourceAccessRole != nil {
			props["ResourceAccessRole"] = aws.ToString(in.ResourceAccessRole)
		}
		return nil
	})
}

func (f *fakeAPI) DeleteProject(ctx context.Context, in *codebuild.DeleteProjectInput, opts ...func(*codebuild.Options)) (*codebuild.DeleteProjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteProject", opts); err != nil {
		return nil, err
	}
	name := aws.ToString(in.Name)
	if f.store.Resource(projectType, name) != nil {
		f.deleting[name] = true
	}
	return &codebuild.DeleteProjectOutput{}, nil
}

func (f *fakeAPI) CreateWebhook(ctx context.Context, in *codebuild.CreateWebhookInput, opts ...func(*codebuild.Options)) (*codebuild.CreateWebhookOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CreateWebhook", opts); err != nil {
		return nil, err
	}
	return &codebuild.CreateWebhookOutput{}, f.change(aws.ToString(in.ProjectName), func(props map[string]any) error {
		if _, ok := webhookIn(props); ok {
			return fault("ResourceAlreadyExistsException")
		}
		triggers := map[string]any{"Webhook": true}
		if err := set(triggers, in, "ProjectName", "ManualCreation"); err != nil {
			return err
		}
		props["Triggers"] = triggers
		return nil
	})
}

func (f *fakeAPI) UpdateWebhook(ctx context.Context, in *codebuild.UpdateWebhookInput, opts ...func(*codebuild.Options)) (*codebuild.UpdateWebhookOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("UpdateWebhook", opts); err != nil {
		return nil, err
	}
	return &codebuild.UpdateWebhookOutput{}, f.change(aws.ToString(in.ProjectName), func(props map[string]any) error {
		triggers, ok := webhookIn(props)
		if !ok {
			return fault(notFound)
		}
		return set(triggers, in, "ProjectName", "RotateSecret")
	})
}

func (f *fakeAPI) DeleteWebhook(ctx context.Context, in *codebuild.DeleteWebhookInput, opts ...func(*codebuild.Options)) (*codebuild.DeleteWebhookOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("DeleteWebhook", opts); err != nil {
		return nil, err
	}
	return &codebuild.DeleteWebhookOutput{}, f.change(aws.ToString(in.ProjectName), func(props map[string]any) error {
		if _, ok := webhookIn(props); !ok {
			return fault(notFound)
		}
		delete(props, "Triggers")
		return nil
	})
}
