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
	"encoding/json"
	"errors"
	"maps"
	"os"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func schema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/aws-codebuild-project.json")
	require.NoError(t, err)
	return string(b)
}

// fakes maps each handler the suite makes to its fake API.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func (f *fakes) of(h native.Handler) *fakeAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.api[h]
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func tags(pairs ...string) []any {
	out := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"Key": pairs[i], "Value": pairs[i+1]})
	}
	return out
}

// triggers returns a webhook that starts a build on the events.
func triggers(events string) map[string]any {
	return map[string]any{
		"Webhook":      true,
		"FilterGroups": []any{[]any{map[string]any{"Type": "EVENT", "Pattern": events}}},
	}
}

func environment(variables ...map[string]any) map[string]any {
	out := map[string]any{
		"Type":        "LINUX_CONTAINER",
		"ComputeType": "BUILD_GENERAL1_SMALL",
		"Image":       "aws/codebuild/standard:7.0",
	}
	if len(variables) > 0 {
		list := []any{}
		for _, v := range variables {
			list = append(list, v)
		}
		out["EnvironmentVariables"] = list
	}
	return out
}

// githubSource is a project source that CodeBuild supports webhooks for.
var githubSource = map[string]any{"Type": "GITHUB", "Location": "https://github.com/example/app.git"}

var project = map[string]any{
	"Name":          "app-build",
	"ServiceRole":   "arn:aws:iam::123456789012:role/app-build",
	"Source":        map[string]any{"Type": "GITHUB", "Location": "https://github.com/example/app.git", "BuildSpec": "version: 0.2"},
	"Artifacts":     map[string]any{"Type": "NO_ARTIFACTS"},
	"Environment":   environment(map[string]any{"Name": "STAGE", "Value": "dev"}),
	"Description":   "Builds the app",
	"EncryptionKey": "alias/aws/s3",
	"Triggers":      triggers("PUSH"),
	"Tags":          tags("team", "a"),
}

func newSuite(fs *fakes, t *testing.T) nativetest.Suite {
	return nativetest.Suite{
		TypeName: projectType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewProject(api), api)
		},
		IdentifierProperty: "Name",
		Identifier:         "app-build",
		API:                projectAPI,
	}
}

func Test_Suite_Project(t *testing.T) {
	var fs fakes
	s := newSuite(&fs, t)
	s.Properties = project
	s.Changed = with(project, map[string]any{"Description": "Builds and tests the app", "TimeoutInMinutes": 30})
	s.Removable = "Triggers"
	s.CreateOnlyChange = with(project, map[string]any{"Name": "other-build"})
	s.OtherForm = map[string]any{
		"EncryptionKey": "arn:aws:kms:us-west-2:123456789012:alias/aws/s3",
		"Environment": with(environment(map[string]any{"Name": "STAGE", "Value": "dev", "Type": "PLAINTEXT"}), map[string]any{
			"ImagePullCredentialsType": "CODEBUILD",
			"PrivilegedMode":           false,
		}),
		"Triggers": map[string]any{
			"Webhook":      true,
			"BuildType":    "BUILD",
			"FilterGroups": []any{[]any{map[string]any{"Type": "EVENT", "Pattern": "PUSH", "ExcludeMatchedPattern": false}}},
		},
	}
	s.MultiCall = with(project, map[string]any{
		"Description": "Builds and tests the app",
		"Visibility":  "PUBLIC_READ",
		"Triggers":    triggers("PUSH, PULL_REQUEST_CREATED"),
	})
	s.Calls = 3
	s.FailCall = nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
		return &fs.of(h).failer
	})
	nativetest.Run(t, s)
}

// Test_Suite_ProjectWebhook runs the suite for updates that create, update, and replace the project's webhook.
func Test_Suite_ProjectWebhook(t *testing.T) {
	scope := func(name string) map[string]any {
		return with(triggers("PUSH"), map[string]any{"ScopeConfiguration": map[string]any{"Name": name, "Scope": "GITHUB_ORGANIZATION"}})
	}
	tests := []struct {
		name       string
		properties map[string]any
		changed    map[string]any
		removable  string
	}{
		{"a webhook is created", with(project, map[string]any{"Triggers": nil}), project, "Description"},
		{"a webhook's filters are removed", project, with(project, map[string]any{"Triggers": map[string]any{"Webhook": true}}), ""},
		{"a new scope replaces the webhook", with(project, map[string]any{"Triggers": scope("example")}), with(project, map[string]any{"Triggers": scope("other")}), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fs fakes
			s := newSuite(&fs, t)
			s.Properties, s.Changed, s.Removable = tt.properties, tt.changed, tt.removable
			nativetest.Run(t, s)
		})
	}
}

// deployed returns a handler whose fake has the project, created through the handler, and the fake.
func deployed(t *testing.T, properties map[string]any) (*Project, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewProject(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	api.takeCalls()
	return h, api
}

// deploy deploys desired over the project as UCP does: it reads the project, computes the patch to desired, which
// removes what lastApplied set and desired doesn't, and calls Update when the patch changes anything.
func deploy(t *testing.T, h *Project, lastApplied map[string]any, desired map[string]any) error {
	name := awsString(desired, "Name")
	current, err := h.Read(t.Context(), region, name)
	require.NoError(t, err)
	currentJSON, err := json.Marshal(current)
	require.NoError(t, err)
	desiredJSON, err := json.Marshal(desired)
	require.NoError(t, err)
	patch, err := awsoperations.GeneratePatchWithOptions(currentJSON, desiredJSON, []byte(schema(t)), awsoperations.PatchOptions{
		Removed:    awsoperations.RemovedPaths(lastApplied, desired),
		Equivalent: h.Options().Equivalent,
	})
	require.NoError(t, err)
	if len(patch) == 0 {
		return nil
	}
	return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: name, Current: current, Desired: desired, Patch: patch})
}

// update calls Update with the patch from current to desired, for an update whose read fails.
func update(t *testing.T, h *Project, current map[string]any, desired map[string]any) error {
	patch, err := jsondiff.Compare(current, desired)
	require.NoError(t, err)
	return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: awsString(desired, "Name"), Current: current, Desired: desired, Patch: patch})
}

func awsString(p map[string]any, name string) string {
	s, _ := p[name].(string)
	return s
}

func Test_Read(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	api.store.Put(projectType, "app-build", map[string]any{"Name": "app-build"})
	h := NewProject(api)

	t.Run("renames and leaves out fields", func(t *testing.T) {
		p, err := properties(&types.Project{
			Name:               aws.String("app-build"),
			Arn:                aws.String(arnPrefix + "app-build"),
			Badge:              &types.ProjectBadge{BadgeEnabled: true, BadgeRequestUrl: aws.String("https://badge")},
			ProjectVisibility:  types.ProjectVisibilityTypePublicRead,
			PublicProjectAlias: aws.String("alias"),
			Source:             &types.ProjectSource{Type: types.SourceTypeNoSource, Buildspec: aws.String("version: 0.2")},
			SecondarySources:   []types.ProjectSource{{Type: types.SourceTypeS3, Buildspec: aws.String("other"), SourceIdentifier: aws.String("s2")}},
			Artifacts:          &types.ProjectArtifacts{Type: types.ArtifactsTypeS3, BucketOwnerAccess: types.BucketOwnerAccessFull},
			SecondaryArtifacts: []types.ProjectArtifacts{{Type: types.ArtifactsTypeS3, ArtifactIdentifier: aws.String("a2"), BucketOwnerAccess: types.BucketOwnerAccessReadOnly}},
			LogsConfig:         &types.LogsConfig{S3Logs: &types.S3LogsConfig{Status: types.LogsConfigStatusTypeDisabled, BucketOwnerAccess: types.BucketOwnerAccessNone}},
			Environment: &types.ProjectEnvironment{
				Type:                 types.EnvironmentTypeLinuxContainer,
				ComputeConfiguration: &types.ComputeConfiguration{VCpu: aws.Int64(2)},
				DockerServer:         &types.DockerServer{ComputeType: types.ComputeTypeBuildGeneral1Small, Status: &types.DockerServerStatus{Status: aws.String("RUNNING")}},
			},
			Webhook: &types.Webhook{
				Url:                    aws.String("https://hook"),
				PayloadUrl:             aws.String("https://payload"),
				Secret:                 aws.String("hook-secret"),
				Status:                 types.WebhookStatusActive,
				BuildType:              types.WebhookBuildTypeBuild,
				FilterGroups:           [][]types.WebhookFilter{{{Type: types.WebhookFilterTypeEvent, Pattern: aws.String("PUSH"), ExcludeMatchedPattern: aws.Bool(false)}}},
				ScopeConfiguration:     &types.ScopeConfiguration{Name: aws.String("example"), Scope: types.WebhookScopeTypeGithubOrganization},
				PullRequestBuildPolicy: &types.PullRequestBuildPolicy{RequiresCommentApproval: types.PullRequestBuildCommentApprovalDisabled},
			},
			Tags: []types.Tag{{Key: aws.String("b"), Value: aws.String("2")}, {Key: aws.String("aws:x"), Value: aws.String("y")}, {Key: aws.String("a"), Value: aws.String("1")}},
		})
		require.NoError(t, err)
		require.Equal(t, map[string]any{
			"Name":               "app-build",
			"Arn":                arnPrefix + "app-build",
			"BadgeEnabled":       true,
			"Visibility":         "PUBLIC_READ",
			"Source":             map[string]any{"Type": "NO_SOURCE", "BuildSpec": "version: 0.2"},
			"SecondarySources":   []any{map[string]any{"Type": "S3", "BuildSpec": "other", "SourceIdentifier": "s2"}},
			"Artifacts":          map[string]any{"Type": "S3"},
			"SecondaryArtifacts": []any{map[string]any{"Type": "S3", "ArtifactIdentifier": "a2"}},
			"LogsConfig":         map[string]any{"S3Logs": map[string]any{"Status": "DISABLED"}},
			"Environment":        map[string]any{"Type": "LINUX_CONTAINER", "DockerServer": map[string]any{"ComputeType": "BUILD_GENERAL1_SMALL"}},
			"Tags":               tags("a", "1", "b", "2"),
			"Triggers": map[string]any{
				"Webhook":                true,
				"BuildType":              "BUILD",
				"FilterGroups":           []any{[]any{map[string]any{"Type": "EVENT", "Pattern": "PUSH", "ExcludeMatchedPattern": false}}},
				"ScopeConfiguration":     map[string]any{"Name": "example", "Scope": "GITHUB_ORGANIZATION"},
				"PullRequestBuildPolicy": map[string]any{"RequiresCommentApproval": "DISABLED"},
			},
		}, p)
	})

	t.Run("a project without a webhook has it off", func(t *testing.T) {
		p, err := h.Read(t.Context(), region, "app-build")
		require.NoError(t, err)
		require.Equal(t, map[string]any{"Webhook": false}, p["Triggers"])
	})

	t.Run("not found", func(t *testing.T) {
		_, err := h.Read(t.Context(), region, "missing")
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("AWS fails", func(t *testing.T) {
		api.errs["BatchGetProjects"] = fault("AccessDeniedException")
		defer delete(api.errs, "BatchGetProjects")
		_, err := h.Read(t.Context(), region, "app-build")
		require.ErrorContains(t, err, "AccessDeniedException")
		_, err = h.Progress(t.Context(), region, "app-build", native.OperationCreate)
		require.ErrorContains(t, err, "AccessDeniedException")
		require.ErrorContains(t, update(t, h, map[string]any{"Name": "app-build"}, map[string]any{"Name": "app-build", "Description": "x"}), "AccessDeniedException")
		require.ErrorContains(t, h.Delete(t.Context(), region, "app-build"), "AccessDeniedException")
	})
}

func Test_Create(t *testing.T) {
	base := map[string]any{"Name": "app-build", "Source": githubSource}

	t.Run("sets visibility and the webhook", func(t *testing.T) {
		properties := with(base, map[string]any{"Visibility": "public_read", "ResourceAccessRole": "arn:aws:iam::123456789012:role/logs", "Triggers": map[string]any{"Webhook": true, "BuildType": "build"}})
		h, api := deployed(t, properties)
		stored := api.store.Resource(projectType, "app-build")
		require.Equal(t, "PUBLIC_READ", stored["Visibility"])
		require.Equal(t, "arn:aws:iam::123456789012:role/logs", stored["ResourceAccessRole"])
		require.Equal(t, map[string]any{"Webhook": true, "BuildType": "BUILD"}, stored["Triggers"])
		read, err := h.Read(t.Context(), region, "app-build")
		require.NoError(t, err)
		require.Equal(t, map[string]any{"Webhook": true, "BuildType": "BUILD"}, read["Triggers"])
	})

	t.Run("a private project and a webhook that is off make one call", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewProject(api)
		name, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(base, map[string]any{"Visibility": "PRIVATE", "Triggers": map[string]any{"Webhook": false}})})
		require.NoError(t, err)
		require.Equal(t, "app-build", name)
		require.Equal(t, []string{"CreateProject"}, api.takeCalls())
	})

	t.Run("a failed webhook leaves the project", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		api.errs["CreateWebhook"] = fault("OAuthProviderException")
		h := NewProject(api)
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(base, map[string]any{"Triggers": triggers("PUSH")})})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.Equal(t, native.OutcomePartial, failure.Outcome)
		require.NotNil(t, api.store.Resource(projectType, "app-build"))
	})

	t.Run("invalid properties", func(t *testing.T) {
		h := NewProject(newFakeAPI(awsfake.NewCloudControl()))
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(base, map[string]any{"TimeoutInMinutes": "an hour"})})
		require.ErrorContains(t, err, "failed to read the properties")
		_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(base, map[string]any{"Triggers": map[string]any{"Webhook": "yes"}})})
		require.ErrorContains(t, err, "failed to read the properties")
	})
}

func Test_Update(t *testing.T) {
	base := map[string]any{"Name": "app-build", "Source": githubSource}

	t.Run("removed properties go back to AWS's defaults", func(t *testing.T) {
		applied := with(base, map[string]any{"TimeoutInMinutes": 30, "ConcurrentBuildLimit": 2, "Cache": map[string]any{"Type": "LOCAL", "Modes": []any{"LOCAL_SOURCE_CACHE"}}, "Tags": tags("team", "a")})
		h, api := deployed(t, applied)
		require.NoError(t, deploy(t, h, applied, base))
		require.Equal(t, []string{"BatchGetProjects", "BatchGetProjects", "UpdateProject"}, api.takeCalls())
		stored := api.store.Resource(projectType, "app-build")
		require.EqualValues(t, 60, stored["TimeoutInMinutes"])
		require.EqualValues(t, -1, stored["ConcurrentBuildLimit"])
		require.Equal(t, map[string]any{"Type": "NO_CACHE"}, stored["Cache"])
		require.NotContains(t, stored, "Tags")
	})

	t.Run("removed visibility makes the project private", func(t *testing.T) {
		applied := with(base, map[string]any{"Visibility": "PUBLIC_READ", "ResourceAccessRole": "arn:aws:iam::123456789012:role/logs"})
		h, api := deployed(t, applied)
		require.NoError(t, deploy(t, h, applied, base))
		require.Equal(t, []string{"BatchGetProjects", "BatchGetProjects", "UpdateProjectVisibility"}, api.takeCalls())
		stored := api.store.Resource(projectType, "app-build")
		require.Equal(t, "PRIVATE", stored["Visibility"])
		require.NotContains(t, stored, "ResourceAccessRole")
	})

	t.Run("a webhook turned off is deleted, and a redeploy only reads", func(t *testing.T) {
		applied := with(base, map[string]any{"Triggers": triggers("PUSH")})
		off := with(base, map[string]any{"Triggers": map[string]any{"Webhook": false}})
		h, api := deployed(t, applied)
		require.NoError(t, deploy(t, h, applied, off))
		require.Equal(t, []string{"BatchGetProjects", "BatchGetProjects", "DeleteWebhook"}, api.takeCalls())
		require.NotContains(t, api.store.Resource(projectType, "app-build"), "Triggers")

		require.NoError(t, deploy(t, h, off, off))
		require.Equal(t, []string{"BatchGetProjects"}, api.takeCalls())

		// Removing Triggers that turned the webhook off changes nothing in AWS.
		require.NoError(t, deploy(t, h, off, base))
		require.Equal(t, []string{"BatchGetProjects", "BatchGetProjects"}, api.takeCalls())
	})

	t.Run("invalid webhook", func(t *testing.T) {
		h, _ := deployed(t, base)
		err := deploy(t, h, base, with(base, map[string]any{"Triggers": map[string]any{"Webhook": "yes"}}))
		require.ErrorContains(t, err, "failed to read the properties")
	})

	t.Run("invalid properties", func(t *testing.T) {
		h, _ := deployed(t, base)
		err := deploy(t, h, base, with(base, map[string]any{"TimeoutInMinutes": "an hour"}))
		require.ErrorContains(t, err, "failed to read the properties")
	})

	t.Run("not found", func(t *testing.T) {
		h := NewProject(newFakeAPI(awsfake.NewCloudControl()))
		require.ErrorIs(t, update(t, h, base, with(base, map[string]any{"Description": "x"})), native.ErrNotFound)
	})
}

func Test_Delete(t *testing.T) {
	base := map[string]any{"Name": "app-build", "Source": githubSource}

	t.Run("deletes the webhook first", func(t *testing.T) {
		h, api := deployed(t, with(base, map[string]any{"Triggers": triggers("PUSH")}))
		require.NoError(t, h.Delete(t.Context(), region, "app-build"))
		require.Equal(t, []string{"BatchGetProjects", "DeleteWebhook", "DeleteProject"}, api.takeCalls())

		progress, err := h.Progress(t.Context(), region, "app-build", native.OperationDelete)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "exists", Waiting: "deleted"}, progress)
		_, err = h.Progress(t.Context(), region, "app-build", native.OperationDelete)
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("a webhook or project already gone is not an error", func(t *testing.T) {
		h, api := deployed(t, with(base, map[string]any{"Triggers": triggers("PUSH")}))
		api.errs["DeleteWebhook"] = fault(notFound)
		api.errs["DeleteProject"] = fault(notFound)
		require.NoError(t, h.Delete(t.Context(), region, "app-build"))
	})

	t.Run("a failed project delete after the webhook's is partly applied", func(t *testing.T) {
		h, api := deployed(t, with(base, map[string]any{"Triggers": triggers("PUSH")}))
		api.errs["DeleteProject"] = fault("InvalidInputException")
		var failure *native.Failure
		require.ErrorAs(t, h.Delete(t.Context(), region, "app-build"), &failure)
		require.Equal(t, native.OutcomePartial, failure.Outcome)
	})

	t.Run("not found", func(t *testing.T) {
		h := NewProject(newFakeAPI(awsfake.NewCloudControl()))
		require.ErrorIs(t, h.Delete(t.Context(), region, "app-build"), native.ErrNotFound)
	})
}

func Test_List(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	api.pageSize = 2
	h := NewProject(api)
	for i := range batchSize + 1 {
		name := "project-" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"Name": name}})
		require.NoError(t, err)
	}
	api.takeCalls()

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Len(t, resources, batchSize+1)
	require.Equal(t, "project-aa", resources[0].Identifier)
	require.Equal(t, "project-aa", resources[0].Properties["Name"])
	calls := api.takeCalls()
	require.Equal(t, 2, countOf(calls, "BatchGetProjects"), "BatchGetProjects takes at most 100 names")
	require.Equal(t, (batchSize+2)/2, countOf(calls, "ListProjects"))

	for _, method := range []string{"ListProjects", "BatchGetProjects"} {
		api.errs[method] = errors.New(method + " failed")
		_, err := h.List(t.Context(), region)
		require.ErrorContains(t, err, method+" failed")
		delete(api.errs, method)
	}
}

func countOf(calls []string, name string) int {
	n := 0
	for _, c := range calls {
		if c == name {
			n++
		}
	}
	return n
}

func Test_Equivalent(t *testing.T) {
	variables := func(items ...map[string]any) []any {
		out := []any{}
		for _, item := range items {
			out = append(out, item)
		}
		return out
	}
	tests := []struct {
		name     string
		path     string
		template any
		aws      any
		want     bool
	}{
		{"default type filled in", "Environment/EnvironmentVariables", variables(map[string]any{"Name": "A", "Value": "1"}), variables(map[string]any{"Name": "A", "Value": "1", "Type": "PLAINTEXT"}), true},
		{"type in lowercase", "Environment/EnvironmentVariables", variables(map[string]any{"Name": "A", "Value": "1", "Type": "parameter_store"}), variables(map[string]any{"Name": "A", "Value": "1", "Type": "PARAMETER_STORE"}), true},
		{"another type", "Environment/EnvironmentVariables", variables(map[string]any{"Name": "A", "Value": "1"}), variables(map[string]any{"Name": "A", "Value": "1", "Type": "SECRETS_MANAGER"}), false},
		{"another value", "Environment/EnvironmentVariables", variables(map[string]any{"Name": "A", "Value": "1"}), variables(map[string]any{"Name": "A", "Value": "2", "Type": "PLAINTEXT"}), false},
		{"value case matters", "Environment/EnvironmentVariables", variables(map[string]any{"Name": "A", "Value": "x"}), variables(map[string]any{"Name": "A", "Value": "X", "Type": "PLAINTEXT"}), false},
		{"template sets the default AWS leaves out", "SecondaryArtifacts", variables(map[string]any{"Type": "S3", "EncryptionDisabled": false}), variables(map[string]any{"Type": "S3"}), true},
		{"template sets a value AWS leaves out", "SecondaryArtifacts", variables(map[string]any{"Type": "S3", "Path": "out"}), variables(map[string]any{"Type": "S3"}), false},
		{"artifact defaults", "SecondaryArtifacts", variables(map[string]any{"Type": "s3", "Location": "bucket"}), variables(map[string]any{"Type": "S3", "Location": "bucket", "NamespaceType": "NONE", "Packaging": "NONE", "EncryptionDisabled": false, "OverrideArtifactName": false}), true},
		{"nested object", "SecondarySources", variables(map[string]any{"Type": "GITHUB", "Auth": map[string]any{"Type": "oauth"}}), variables(map[string]any{"Type": "GITHUB", "Auth": map[string]any{"Type": "OAUTH"}, "InsecureSsl": false}), true},
		{"nested object with another field", "SecondarySources", variables(map[string]any{"Type": "GITHUB", "Auth": map[string]any{"Type": "OAUTH"}}), variables(map[string]any{"Type": "GITHUB", "Auth": map[string]any{"Type": "OAUTH", "Resource": "x"}}), false},
		{"filter defaults", "Triggers/FilterGroups", []any{[]any{map[string]any{"Type": "event", "Pattern": "PUSH"}}}, []any{[]any{map[string]any{"Type": "EVENT", "Pattern": "PUSH", "ExcludeMatchedPattern": false}}}, true},
		{"another filter", "Triggers/FilterGroups", []any{[]any{map[string]any{"Type": "EVENT", "Pattern": "PUSH"}}}, []any{[]any{map[string]any{"Type": "EVENT", "Pattern": "PUSH", "ExcludeMatchedPattern": true}}}, false},
		{"another number of filters", "Triggers/FilterGroups", []any{[]any{map[string]any{"Type": "EVENT", "Pattern": "PUSH"}}}, []any{[]any{}}, false},
		{"another length", "SecondarySources", variables(map[string]any{"Type": "S3"}), variables(), false},
		{"not a list", "SecondarySources", "S3", "S3", false},
		{"another path", "Description", "a", "A", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, equivalent(tt.path, tt.template, tt.aws))
		})
	}
}
