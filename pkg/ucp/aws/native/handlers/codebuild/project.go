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

// Package codebuild has the native handler for AWS CodeBuild projects, which Cloud Control can't create in some
// regions.
package codebuild

import (
	"context"
	"reflect"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	"github.com/aws/aws-sdk-go-v2/service/codebuild/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	projectType = "AWS::CodeBuild::Project"

	// notFound is the error code CodeBuild returns for a project or webhook that doesn't exist.
	notFound = "ResourceNotFoundException"

	// batchSize is how many projects BatchGetProjects takes in one call.
	batchSize = 100
)

// API is the part of the CodeBuild client the handler uses.
type API interface {
	CreateProject(ctx context.Context, params *codebuild.CreateProjectInput, optFns ...func(*codebuild.Options)) (*codebuild.CreateProjectOutput, error)
	BatchGetProjects(ctx context.Context, params *codebuild.BatchGetProjectsInput, optFns ...func(*codebuild.Options)) (*codebuild.BatchGetProjectsOutput, error)
	ListProjects(ctx context.Context, params *codebuild.ListProjectsInput, optFns ...func(*codebuild.Options)) (*codebuild.ListProjectsOutput, error)
	UpdateProject(ctx context.Context, params *codebuild.UpdateProjectInput, optFns ...func(*codebuild.Options)) (*codebuild.UpdateProjectOutput, error)
	UpdateProjectVisibility(ctx context.Context, params *codebuild.UpdateProjectVisibilityInput, optFns ...func(*codebuild.Options)) (*codebuild.UpdateProjectVisibilityOutput, error)
	DeleteProject(ctx context.Context, params *codebuild.DeleteProjectInput, optFns ...func(*codebuild.Options)) (*codebuild.DeleteProjectOutput, error)
	CreateWebhook(ctx context.Context, params *codebuild.CreateWebhookInput, optFns ...func(*codebuild.Options)) (*codebuild.CreateWebhookOutput, error)
	UpdateWebhook(ctx context.Context, params *codebuild.UpdateWebhookInput, optFns ...func(*codebuild.Options)) (*codebuild.UpdateWebhookOutput, error)
	DeleteWebhook(ctx context.Context, params *codebuild.DeleteWebhookInput, optFns ...func(*codebuild.Options)) (*codebuild.DeleteWebhookOutput, error)
}

var inRegion = awsapi.InRegion[codebuild.Options]

var _ native.Handler = (*Project)(nil)

// Project serves AWS::CodeBuild::Project. Its identifier is Name, which UCP generates when the template leaves it
// out. Every call takes effect at once. Visibility and ResourceAccessRole change through UpdateProjectVisibility, and
// Triggers through the project's webhook, so a create or update that sets them makes more than one call.
type Project struct {
	api API
}

// NewProject returns the handler for CodeBuild projects.
func NewProject(api API) *Project {
	return &Project{api: api}
}

// TypeName returns AWS::CodeBuild::Project.
func (h *Project) TypeName() string {
	return projectType
}

// Options returns how UCP uses the handler.
func (h *Project) Options() native.Options {
	return native.Options{
		List:         native.ListRegion,
		Recovery:     native.RecoverByIdentifier,
		NameProperty: "Name",
		Equivalent:   equivalent,
	}
}

// itemDefaults are the values AWS fills in for fields a template leaves out of the items of these lists, including
// the items of the lists in them. A list is compared as a whole, so a filled-in field would otherwise count as a
// change.
var itemDefaults = map[string]map[string]any{
	"Environment/EnvironmentVariables": {"Type": "PLAINTEXT"},
	"Triggers/FilterGroups":            {"ExcludeMatchedPattern": false},
	"SecondaryArtifacts":               {"EncryptionDisabled": false, "NamespaceType": "NONE", "OverrideArtifactName": false, "Packaging": "NONE"},
	"SecondarySources":                 {"InsecureSsl": false, "ReportBuildStatus": false},
}

// caseless are the fields of list items whose values AWS returns in uppercase, as the schema's propertyTransform says.
var caseless = []string{"Type", "NamespaceType", "Packaging"}

// equivalent treats a list AWS returns with defaults filled into its items, and enum values in uppercase, as the
// template's list.
func equivalent(path string, template any, aws any) bool {
	defaults, ok := itemDefaults[path]
	if !ok {
		return false
	}
	t, okT := template.([]any)
	a, okA := aws.([]any)
	if !okT || !okA || len(t) != len(a) {
		return false
	}
	for i := range t {
		if !sameField("", t[i], a[i], defaults) {
			return false
		}
	}
	return true
}

// sameField reports whether the template's value of the field key matches AWS's. In an object, a field only one side
// has must hold its default.
func sameField(key string, template any, aws any, defaults map[string]any) bool {
	if tl, ok := template.([]any); ok {
		al, ok := aws.([]any)
		if !ok || len(tl) != len(al) {
			return false
		}
		for i := range tl {
			if !sameField(key, tl[i], al[i], defaults) {
				return false
			}
		}
		return true
	}
	t, okT := template.(map[string]any)
	a, okA := aws.(map[string]any)
	if okT && okA {
		for k, av := range a {
			tv, ok := t[k]
			if !ok {
				tv = defaults[k]
			}
			if !sameField(k, tv, av, nil) {
				return false
			}
		}
		for k, tv := range t {
			if _, ok := a[k]; !ok && !sameField(k, tv, defaults[k], nil) {
				return false
			}
		}
		return true
	}
	ts, okT := template.(string)
	as, okA := aws.(string)
	if okT && okA && slices.Contains(caseless, key) {
		return strings.EqualFold(ts, as)
	}
	return reflect.DeepEqual(template, aws)
}

// describe returns the project with the name, or ErrNotFound.
func (h *Project) describe(ctx context.Context, region string, name string) (*types.Project, error) {
	out, err := h.api.BatchGetProjects(ctx, &codebuild.BatchGetProjectsInput{Names: []string{name}}, inRegion(region))
	if err != nil {
		return nil, err
	}
	if len(out.Projects) == 0 {
		return nil, native.ErrNotFound
	}
	return &out.Projects[0], nil
}

// Read returns the project's properties.
func (h *Project) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	project, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return properties(project)
}

// properties returns the project's properties, with the schema's names where they differ from the API's, and without
// fields the schema doesn't have. Triggers describes the project's webhook, with Webhook false when it has none, so a
// template that removes Triggers deletes the webhook.
func properties(project *types.Project) (map[string]any, error) {
	p, err := awsapi.EncodeObject(project)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"Badge", "Created", "LastModified", "ProjectVisibility", "PublicProjectAlias", "Tags", "Webhook"} {
		delete(p, name)
	}
	p["BadgeEnabled"] = project.Badge != nil && project.Badge.BadgeEnabled
	if project.ProjectVisibility != "" {
		p["Visibility"] = string(project.ProjectVisibility)
	}
	if tags := awsapi.TagListOf(project.Tags); tags != nil {
		p["Tags"] = tags
	}
	if p["Triggers"], err = triggersOf(project.Webhook); err != nil {
		return nil, err
	}
	for _, source := range append(objects(p, "SecondarySources"), object(p, "Source")) {
		if spec, ok := source["Buildspec"]; ok {
			delete(source, "Buildspec")
			source["BuildSpec"] = spec
		}
	}
	for _, artifacts := range append(objects(p, "SecondaryArtifacts"), object(p, "Artifacts")) {
		delete(artifacts, "BucketOwnerAccess")
	}
	delete(object(object(p, "LogsConfig"), "S3Logs"), "BucketOwnerAccess")
	environment := object(p, "Environment")
	delete(environment, "ComputeConfiguration")
	delete(object(environment, "DockerServer"), "Status")
	return p, nil
}

// object returns the object property name of p, or nil.
func object(p map[string]any, name string) map[string]any {
	m, _ := p[name].(map[string]any)
	return m
}

// objects returns the objects in the list property name of p.
func objects(p map[string]any, name string) []map[string]any {
	list, _ := p[name].([]any)
	out := []map[string]any{}
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// separate are the properties CreateProject doesn't take. Arn is read-only.
var separate = []string{"Arn", "ResourceAccessRole", "Tags", "Triggers", "Visibility"}

// Create creates the project, then sets its visibility and creates its webhook when the template asks for them.
func (h *Project) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	name := awsapi.String(p, "Name")
	opt := inRegion(request.Region)
	input := &codebuild.CreateProjectInput{}
	if err := awsapi.Decode(awsapi.Without(p, separate...), input); err != nil {
		return "", err
	}
	input.Tags = awsapi.SDKTags[types.Tag](awsapi.Tags(p))
	want, err := webhookOf(p)
	if err != nil {
		return "", err
	}

	var arn *string
	calls := []func() error{func() error {
		out, err := h.api.CreateProject(ctx, input, opt)
		if err == nil && out.Project != nil {
			arn = out.Project.Arn
		}
		return err
	}}
	visibility := awsapi.String(p, "Visibility")
	if awsapi.String(p, "ResourceAccessRole") != "" || (visibility != "" && !strings.EqualFold(visibility, string(types.ProjectVisibilityTypePrivate))) {
		calls = append(calls, func() error { return h.setVisibility(ctx, opt, arn, p) })
	}
	if want != nil {
		calls = append(calls, func() error { return h.createWebhook(ctx, opt, name, want) })
	}
	if err := awsapi.Calls(calls...); err != nil {
		return "", err
	}
	return name, nil
}

// setVisibility sets the project's visibility and resource access role to the values in p. A template that leaves
// out Visibility makes the project private.
func (h *Project) setVisibility(ctx context.Context, opt func(*codebuild.Options), arn *string, p map[string]any) error {
	input := &codebuild.UpdateProjectVisibilityInput{
		ProjectArn:        arn,
		ProjectVisibility: types.ProjectVisibilityTypePrivate,
	}
	if visibility := awsapi.String(p, "Visibility"); visibility != "" {
		input.ProjectVisibility = types.ProjectVisibilityType(strings.ToUpper(visibility))
	}
	if role := awsapi.String(p, "ResourceAccessRole"); role != "" {
		input.ResourceAccessRole = aws.String(role)
	}
	_, err := h.api.UpdateProjectVisibility(ctx, input, opt)
	return err
}

// webhook is the Triggers property: the webhook that starts a build when code is pushed.
type webhook struct {
	Webhook                bool
	BuildType              types.WebhookBuildType
	FilterGroups           [][]types.WebhookFilter
	ScopeConfiguration     *types.ScopeConfiguration
	PullRequestBuildPolicy *types.PullRequestBuildPolicy
}

// triggersOf returns the Triggers property for the project's webhook, which is off when w is nil.
func triggersOf(w *types.Webhook) (map[string]any, error) {
	if w == nil {
		return map[string]any{"Webhook": false}, nil
	}
	return awsapi.EncodeObject(webhook{
		Webhook:                true,
		BuildType:              w.BuildType,
		FilterGroups:           w.FilterGroups,
		ScopeConfiguration:     w.ScopeConfiguration,
		PullRequestBuildPolicy: w.PullRequestBuildPolicy,
	})
}

// webhookOf returns the webhook p asks for, or nil when it asks for none.
func webhookOf(p map[string]any) (*webhook, error) {
	triggers, ok := p["Triggers"].(map[string]any)
	if !ok {
		return nil, nil
	}
	w := &webhook{}
	if err := awsapi.Decode(triggers, w); err != nil {
		return nil, err
	}
	if !w.Webhook {
		return nil, nil
	}
	w.BuildType = types.WebhookBuildType(strings.ToUpper(string(w.BuildType)))
	return w, nil
}

func (h *Project) createWebhook(ctx context.Context, opt func(*codebuild.Options), name string, w *webhook) error {
	_, err := h.api.CreateWebhook(ctx, &codebuild.CreateWebhookInput{
		ProjectName:            aws.String(name),
		BuildType:              w.BuildType,
		FilterGroups:           w.FilterGroups,
		ScopeConfiguration:     w.ScopeConfiguration,
		PullRequestBuildPolicy: w.PullRequestBuildPolicy,
	}, opt)
	return err
}

// deleteWebhook deletes the project's webhook. A webhook that is already gone is not an error.
func (h *Project) deleteWebhook(ctx context.Context, opt func(*codebuild.Options), name string) error {
	_, err := h.api.DeleteWebhook(ctx, &codebuild.DeleteWebhookInput{ProjectName: aws.String(name)}, opt)
	if awsapi.HasCode(err, notFound) {
		return nil
	}
	return err
}

// webhookCalls returns the calls that change the project's webhook from current to the one desired asks for.
// UpdateWebhook can't change the scope configuration, so a new scope replaces the webhook.
func (h *Project) webhookCalls(ctx context.Context, opt func(*codebuild.Options), name string, current *types.Webhook, desired map[string]any) ([]func() error, error) {
	want, err := webhookOf(desired)
	if err != nil {
		return nil, err
	}
	remove := func() error { return h.deleteWebhook(ctx, opt, name) }
	create := func() error { return h.createWebhook(ctx, opt, name, want) }
	switch {
	case want == nil && current == nil:
		return nil, nil
	case want == nil:
		return []func() error{remove}, nil
	case current == nil:
		return []func() error{create}, nil
	}
	currentScope, err := awsapi.Encode(current.ScopeConfiguration)
	if err != nil {
		return nil, err
	}
	wantScope, err := awsapi.Encode(want.ScopeConfiguration)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(currentScope, wantScope) {
		return []func() error{remove, create}, nil
	}
	filters := want.FilterGroups
	if filters == nil {
		filters = [][]types.WebhookFilter{}
	}
	return []func() error{func() error {
		_, err := h.api.UpdateWebhook(ctx, &codebuild.UpdateWebhookInput{
			ProjectName:            aws.String(name),
			BuildType:              want.BuildType,
			FilterGroups:           filters,
			PullRequestBuildPolicy: want.PullRequestBuildPolicy,
		}, opt)
		return err
	}}, nil
}

// updatable are the properties UpdateProject changes. Tags change through it too.
var updatable = []string{
	"Artifacts", "AutoRetryLimit", "BadgeEnabled", "BuildBatchConfig", "Cache", "ConcurrentBuildLimit", "Description",
	"EncryptionKey", "Environment", "FileSystemLocations", "LogsConfig", "QueuedTimeoutInMinutes", "SecondaryArtifacts",
	"SecondarySourceVersions", "SecondarySources", "ServiceRole", "Source", "SourceVersion", "TimeoutInMinutes",
	"VpcConfig",
}

// removedDefaults are the values a property goes back to when the template removes it: AWS's defaults, and empty
// values that clear it. The required properties Artifacts, Environment, ServiceRole, and Source can't be removed.
var removedDefaults = map[string]any{
	"AutoRetryLimit":          0,
	"BadgeEnabled":            false,
	"BuildBatchConfig":        map[string]any{},
	"Cache":                   map[string]any{"Type": string(types.CacheTypeNoCache)},
	"ConcurrentBuildLimit":    -1,
	"Description":             "",
	"EncryptionKey":           "alias/aws/s3",
	"FileSystemLocations":     []any{},
	"LogsConfig":              map[string]any{"CloudWatchLogs": map[string]any{"Status": "ENABLED"}, "S3Logs": map[string]any{"Status": "DISABLED"}},
	"QueuedTimeoutInMinutes":  480,
	"SecondaryArtifacts":      []any{},
	"SecondarySourceVersions": []any{},
	"SecondarySources":        []any{},
	"SourceVersion":           "",
	"TimeoutInMinutes":        60,
	"VpcConfig":               map[string]any{},
}

// Update makes the patch's changes with UpdateProject, then UpdateProjectVisibility, then the webhook's calls, each
// only when the patch changes its properties.
func (h *Project) Update(ctx context.Context, request native.UpdateRequest) error {
	project, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	changed := awsapi.Changed(request)
	values := map[string]any{}
	for _, name := range changed {
		if !slices.Contains(updatable, name) {
			continue
		}
		value, ok := request.Desired[name]
		if !ok {
			value = removedDefaults[name]
		}
		values[name] = value
	}

	calls := []func() error{}
	if len(values) > 0 || slices.Contains(changed, "Tags") {
		input := &codebuild.UpdateProjectInput{}
		if err := awsapi.Decode(values, input); err != nil {
			return err
		}
		input.Name = aws.String(request.Identifier)
		if slices.Contains(changed, "Tags") {
			input.Tags = awsapi.SDKTags[types.Tag](awsapi.Tags(request.Desired))
		}
		calls = append(calls, func() error {
			_, err := h.api.UpdateProject(ctx, input, opt)
			return err
		})
	}
	if slices.Contains(changed, "Visibility") || slices.Contains(changed, "ResourceAccessRole") {
		calls = append(calls, func() error { return h.setVisibility(ctx, opt, project.Arn, request.Desired) })
	}
	if slices.Contains(changed, "Triggers") {
		webhookCalls, err := h.webhookCalls(ctx, opt, request.Identifier, project.Webhook, request.Desired)
		if err != nil {
			return err
		}
		calls = append(calls, webhookCalls...)
	}
	return awsapi.Calls(calls...)
}

// Delete deletes the project's webhook, so the source provider stops sending events, then the project. A project
// that is gone by the time of the delete call is not an error: Progress then finds it gone.
func (h *Project) Delete(ctx context.Context, region string, identifier string) error {
	project, err := h.describe(ctx, region, identifier)
	if err != nil {
		return err
	}
	opt := inRegion(region)
	calls := []func() error{}
	if project.Webhook != nil {
		calls = append(calls, func() error { return h.deleteWebhook(ctx, opt, identifier) })
	}
	calls = append(calls, func() error {
		_, err := h.api.DeleteProject(ctx, &codebuild.DeleteProjectInput{Name: aws.String(identifier)}, opt)
		if awsapi.HasCode(err, notFound) {
			return nil
		}
		return err
	})
	return awsapi.Calls(calls...)
}

// List returns every project in the region.
func (h *Project) List(ctx context.Context, region string) ([]native.Resource, error) {
	opt := inRegion(region)
	names := []string{}
	input := &codebuild.ListProjectsInput{}
	for {
		page, err := h.api.ListProjects(ctx, input, opt)
		if err != nil {
			return nil, err
		}
		names = append(names, page.Projects...)
		if page.NextToken == nil {
			break
		}
		input.NextToken = page.NextToken
	}
	out := []native.Resource{}
	for batch := range slices.Chunk(names, batchSize) {
		page, err := h.api.BatchGetProjects(ctx, &codebuild.BatchGetProjectsInput{Names: batch}, opt)
		if err != nil {
			return nil, err
		}
		for i := range page.Projects {
			p, err := properties(&page.Projects[i])
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: aws.ToString(page.Projects[i].Name), Properties: p})
		}
	}
	return out, nil
}

// Progress reads the project. Every call takes effect at once, so a create or update is done once the project
// exists, and a delete once it is gone.
func (h *Project) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if _, err := h.describe(ctx, region, identifier); err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{State: "exists", Waiting: "deleted"}, nil
	}
	return native.Progress{Done: true}, nil
}
