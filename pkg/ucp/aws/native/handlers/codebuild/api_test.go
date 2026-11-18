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
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	"github.com/aws/aws-sdk-go-v2/service/codebuild/types"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	apiReference       = "https://docs.aws.amazon.com/codebuild/latest/APIReference/"
	projectDoc         = apiReference + "API_Project.html"
	projectSourceDoc   = apiReference + "API_ProjectSource.html"
	projectArtifactDoc = apiReference + "API_ProjectArtifacts.html"
	environmentDoc     = apiReference + "API_ProjectEnvironment.html"
	webhookFilterDoc   = apiReference + "API_WebhookFilter.html"
	scopeDoc           = apiReference + "API_ScopeConfiguration.html"
	userGuide          = "https://docs.aws.amazon.com/codebuild/latest/userguide/"

	// limitsUnclear is what CodeBuild's pages leave unclear about the error for a value outside a limit.
	limitsUnclear = "The page lists InvalidInputException for input that isn't valid and names no other code for its " +
		"value limits, while the Common Errors page lists ValidationError for input that breaks a constraint. The fake " +
		"returns InvalidInputException for every limit."
)

// projectInput returns the input of a CreateProject call for a valid project named app-build, built from a GitHub
// repository.
func projectInput() *codebuild.CreateProjectInput {
	return &codebuild.CreateProjectInput{
		Name:        aws.String("app-build"),
		ServiceRole: aws.String("arn:aws:iam::123456789012:role/app-build"),
		Source:      &types.ProjectSource{Type: types.SourceTypeGithub, Location: aws.String("https://github.com/example/app.git")},
		Artifacts:   &types.ProjectArtifacts{Type: types.ArtifactsTypeNoArtifacts},
		Environment: &types.ProjectEnvironment{
			Type:        types.EnvironmentTypeLinuxContainer,
			ComputeType: types.ComputeTypeBuildGeneral1Small,
			Image:       aws.String("aws/codebuild/standard:7.0"),
		},
	}
}

// createProject creates app-build with a fake CodeBuild API backed by store, after edit changes the input.
func createProject(ctx context.Context, store *awsfake.CloudControl, edit func(in *codebuild.CreateProjectInput)) error {
	in := projectInput()
	if edit != nil {
		edit(in)
	}
	_, err := newFakeAPI(store).CreateProject(ctx, in, inRegion(region))
	return err
}

// readProject returns app-build, as BatchGetProjects returns it.
func readProject(ctx context.Context, store *awsfake.CloudControl) (types.Project, error) {
	out, err := newFakeAPI(store).BatchGetProjects(ctx, &codebuild.BatchGetProjectsInput{Names: []string{"app-build"}}, inRegion(region))
	if err != nil {
		return types.Project{}, err
	}
	if len(out.Projects) != 1 {
		return types.Project{}, fmt.Errorf("found %d projects", len(out.Projects))
	}
	return out.Projects[0], nil
}

// breakCreate returns a Break that creates app-build after edit changes the input.
func breakCreate(edit func(in *codebuild.CreateProjectInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		return createProject(ctx, store, edit)
	}
}

// breakUpdate returns a Break that creates app-build, then updates it with the input edit sets.
func breakUpdate(edit func(in *codebuild.UpdateProjectInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		if err := createProject(ctx, store, nil); err != nil {
			return err
		}
		in := &codebuild.UpdateProjectInput{Name: aws.String("app-build")}
		edit(in)
		_, err := newFakeAPI(store).UpdateProject(ctx, in, inRegion(region))
		return err
	}
}

// pushFilters are filter groups that start a build on a push.
func pushFilters() [][]types.WebhookFilter {
	return [][]types.WebhookFilter{{{Type: types.WebhookFilterTypeEvent, Pattern: aws.String("PUSH")}}}
}

// breakCreateWebhook returns a Break that creates app-build after editProject changes its input, then creates its
// webhook with the input edit sets.
func breakCreateWebhook(editProject func(in *codebuild.CreateProjectInput), edit func(in *codebuild.CreateWebhookInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		if err := createProject(ctx, store, editProject); err != nil {
			return err
		}
		in := &codebuild.CreateWebhookInput{ProjectName: aws.String("app-build"), FilterGroups: pushFilters()}
		edit(in)
		_, err := newFakeAPI(store).CreateWebhook(ctx, in, inRegion(region))
		return err
	}
}

// breakUpdateWebhook returns a Break that creates app-build after editProject changes its input, and its webhook
// with buildType, then updates the webhook with the input edit sets.
func breakUpdateWebhook(editProject func(in *codebuild.CreateProjectInput), buildType types.WebhookBuildType, edit func(in *codebuild.UpdateWebhookInput)) func(ctx context.Context, store *awsfake.CloudControl) error {
	return func(ctx context.Context, store *awsfake.CloudControl) error {
		if err := createProject(ctx, store, editProject); err != nil {
			return err
		}
		f := newFakeAPI(store)
		if _, err := f.CreateWebhook(ctx, &codebuild.CreateWebhookInput{ProjectName: aws.String("app-build"), BuildType: buildType, FilterGroups: pushFilters()}, inRegion(region)); err != nil {
			return err
		}
		in := &codebuild.UpdateWebhookInput{ProjectName: aws.String("app-build"), FilterGroups: pushFilters()}
		edit(in)
		_, err := f.UpdateWebhook(ctx, in, inRegion(region))
		return err
	}
}

// filterRules returns the rules WebhookFilter's page gives for the filter groups of a webhook call, which call makes
// with the filter groups.
func filterRules(call func(groups [][]types.WebhookFilter) func(ctx context.Context, store *awsfake.CloudControl) error) []nativetest.Rule {
	return []nativetest.Rule{
		{
			Rule:  "a filter group has an EVENT filter",
			Doc:   apiReference + "API_UpdateWebhook.html",
			Break: call([][]types.WebhookFilter{{{Type: types.WebhookFilterTypeHeadRef, Pattern: aws.String("^refs/heads/main$")}}}),
			Code:  invalidInput,
		},
		{
			Rule: "a filter's type is one of EVENT, ACTOR_ACCOUNT_ID, HEAD_REF, BASE_REF, FILE_PATH, COMMIT_MESSAGE, " +
				"TAG_NAME, RELEASE_NAME, REPOSITORY_NAME, ORGANIZATION_NAME, and WORKFLOW_NAME",
			Doc: webhookFilterDoc,
			Break: call([][]types.WebhookFilter{{
				{Type: types.WebhookFilterTypeEvent, Pattern: aws.String("PUSH")},
				{Type: "UNKNOWN", Pattern: aws.String("main")},
			}}),
			Code: invalidInput,
		},
		{
			Rule:  "an EVENT filter's pattern lists webhook events, such as PUSH and PULL_REQUEST_CREATED",
			Doc:   webhookFilterDoc,
			Break: call([][]types.WebhookFilter{{{Type: types.WebhookFilterTypeEvent, Pattern: aws.String("PUSH, COMMIT")}}}),
			Code:  invalidInput,
		},
		{
			Rule: "another filter's pattern is an RE2 regular expression, without lookahead, lookbehind, or backreferences",
			Doc:  webhookFilterDoc,
			Break: call([][]types.WebhookFilter{{
				{Type: types.WebhookFilterTypeEvent, Pattern: aws.String("PUSH")},
				{Type: types.WebhookFilterTypeHeadRef, Pattern: aws.String("^refs/heads/(?!main)")},
			}}),
			Code: invalidInput,
		},
	}
}

// webhookNameRule is the rule of the webhook calls for the project name, which call makes with name.
func webhookNameRule(call func(ctx context.Context, store *awsfake.CloudControl, name string) error) nativetest.Rule {
	return nativetest.Rule{
		Rule: "ProjectName is 2 to 150 characters of [A-Za-z0-9-_], starting with a letter or digit",
		Break: func(ctx context.Context, store *awsfake.CloudControl) error {
			return call(ctx, store, "-app-build")
		},
		Code: invalidInput,
	}
}

// projectAPI records the CodeBuild calls Project makes.
var projectAPI = map[string]nativetest.Call{
	"BatchGetProjects": {
		Doc: apiReference + "API_BatchGetProjects.html",
		Rules: []nativetest.Rule{
			{
				Rule: "Names has 1 to 100 names",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					names := []string{}
					for i := range 101 {
						names = append(names, "project-"+strconv.Itoa(i))
					}
					_, err := newFakeAPI(store).BatchGetProjects(ctx, &codebuild.BatchGetProjectsInput{Names: names}, inRegion(region))
					return err
				},
				Code: invalidInput,
			},
			{
				Rule: "each name has at least 1 character",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).BatchGetProjects(ctx, &codebuild.BatchGetProjectsInput{Names: []string{""}}, inRegion(region))
					return err
				},
				Code: invalidInput,
			},
			{
				Rule: "Names takes a project's ARN as well as its name, and lists a project that isn't found in ProjectsNotFound",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := createProject(ctx, store, nil); err != nil {
						return err
					}
					out, err := newFakeAPI(store).BatchGetProjects(ctx, &codebuild.BatchGetProjectsInput{Names: []string{arnPrefix + "app-build", "missing"}}, inRegion(region))
					if err != nil {
						return err
					}
					if len(out.Projects) != 1 || aws.ToString(out.Projects[0].Name) != "app-build" || len(out.ProjectsNotFound) != 1 || out.ProjectsNotFound[0] != "missing" {
						return fmt.Errorf("the fake found %d projects, and didn't find %v", len(out.Projects), out.ProjectsNotFound)
					}
					return nil
				},
			},
		},
		Unclear: "The page doesn't say how soon a read stops returning a deleted project. The fake returns it in one " +
			"more read, so the handler's delete waits for it to go.",
	},
	"CreateProject": {
		Doc: apiReference + "API_CreateProject.html",
		Rules: []nativetest.Rule{
			{
				Rule:  "Name is 2 to 150 characters of [A-Za-z0-9-_], starting with a letter or digit",
				Break: breakCreate(func(in *codebuild.CreateProjectInput) { in.Name = aws.String("app build") }),
				Code:  invalidInput,
			},
			{
				Rule: "project names are unique in the account",
				Doc:  userGuide + "create-project.html",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := createProject(ctx, store, nil); err != nil {
						return err
					}
					return createProject(ctx, store, nil)
				},
				Code: "ResourceAlreadyExistsException",
			},
			{
				Rule:  "Description has at most 255 characters",
				Break: breakCreate(func(in *codebuild.CreateProjectInput) { in.Description = aws.String(strings.Repeat("a", 256)) }),
				Code:  invalidInput,
			},
			{
				Rule: "Description's limit counts characters, not bytes",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					return createProject(ctx, store, func(in *codebuild.CreateProjectInput) { in.Description = aws.String(strings.Repeat("é", 255)) })
				},
			},
			{
				Rule:  "TimeoutInMinutes is 5 to 2160",
				Break: breakCreate(func(in *codebuild.CreateProjectInput) { in.TimeoutInMinutes = aws.Int32(4) }),
				Code:  invalidInput,
			},
			{
				Rule:  "QueuedTimeoutInMinutes is 5 to 480",
				Break: breakCreate(func(in *codebuild.CreateProjectInput) { in.QueuedTimeoutInMinutes = aws.Int32(481) }),
				Code:  invalidInput,
			},
			{
				Rule: "SecondarySources, SecondaryArtifacts, and SecondarySourceVersions have at most 12 items, and Tags at most 50",
				Break: breakCreate(func(in *codebuild.CreateProjectInput) {
					for i := range 13 {
						in.SecondaryArtifacts = append(in.SecondaryArtifacts, types.ProjectArtifacts{Type: types.ArtifactsTypeNoArtifacts, ArtifactIdentifier: aws.String("a" + strconv.Itoa(i))})
					}
				}),
				Code: invalidInput,
			},
			{
				Rule: "a project that uses a fleet sets no VpcConfig",
				Break: breakCreate(func(in *codebuild.CreateProjectInput) {
					in.Environment.Fleet = &types.ProjectFleet{FleetArn: aws.String("arn:aws:codebuild:us-west-2:123456789012:fleet/app:1")}
					in.VpcConfig = &types.VpcConfig{VpcId: aws.String("vpc-1")}
				}),
				Code: invalidInput,
			},
			{
				Rule: "ReportBuildStatus is only for GitHub, GitHub Enterprise, GitLab, GitLab Self Managed, and Bitbucket sources",
				Doc:  projectSourceDoc,
				Break: breakCreate(func(in *codebuild.CreateProjectInput) {
					in.Source = &types.ProjectSource{Type: types.SourceTypeS3, Location: aws.String("bucket/app.zip"), ReportBuildStatus: aws.Bool(true)}
				}),
				Code: invalidInput,
			},
			{
				Rule: "EncryptionDisabled is only for S3 artifacts",
				Doc:  projectArtifactDoc,
				Break: breakCreate(func(in *codebuild.CreateProjectInput) {
					in.Artifacts.EncryptionDisabled = aws.Bool(true)
				}),
				Code: invalidInput,
			},
			{
				Rule: "secondary artifacts can't be of type CODEPIPELINE",
				Doc:  projectArtifactDoc,
				Break: breakCreate(func(in *codebuild.CreateProjectInput) {
					in.SecondaryArtifacts = []types.ProjectArtifacts{{Type: types.ArtifactsTypeCodepipeline, ArtifactIdentifier: aws.String("out")}}
				}),
				Code: invalidInput,
			},
			{
				Rule: "TimeoutInMinutes defaults to 60",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := createProject(ctx, store, nil); err != nil {
						return err
					}
					p, err := readProject(ctx, store)
					if err != nil {
						return err
					}
					if aws.ToInt32(p.TimeoutInMinutes) != 60 {
						return fmt.Errorf("the timeout is %d", aws.ToInt32(p.TimeoutInMinutes))
					}
					return nil
				},
			},
			{
				Rule: "a project that sets no EncryptionKey uses the AWS managed key for Amazon S3",
				Doc:  projectDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := createProject(ctx, store, nil); err != nil {
						return err
					}
					p, err := readProject(ctx, store)
					if err != nil {
						return err
					}
					if !strings.HasSuffix(aws.ToString(p.EncryptionKey), ":alias/aws/s3") {
						return fmt.Errorf("the key is %q", aws.ToString(p.EncryptionKey))
					}
					return nil
				},
			},
			{
				Rule: "PrivilegedMode defaults to false",
				Doc:  environmentDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := createProject(ctx, store, nil); err != nil {
						return err
					}
					p, err := readProject(ctx, store)
					if err != nil {
						return err
					}
					if p.Environment.PrivilegedMode == nil || *p.Environment.PrivilegedMode {
						return fmt.Errorf("the environment is %+v", p.Environment)
					}
					return nil
				},
			},
			{
				Rule: "S3 artifacts default NamespaceType and Packaging to NONE",
				Doc:  projectArtifactDoc,
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					err := createProject(ctx, store, func(in *codebuild.CreateProjectInput) {
						in.SecondaryArtifacts = []types.ProjectArtifacts{{Type: types.ArtifactsTypeS3, Location: aws.String("bucket"), ArtifactIdentifier: aws.String("out")}}
					})
					if err != nil {
						return err
					}
					p, err := readProject(ctx, store)
					if err != nil {
						return err
					}
					a := p.SecondaryArtifacts[0]
					if a.NamespaceType != types.ArtifactNamespaceNone || a.Packaging != types.ArtifactPackagingNone {
						return fmt.Errorf("the artifacts are %+v", a)
					}
					return nil
				},
			},
		},
		Unclear: limitsUnclear + " The page marks Artifacts, Environment, Name, ServiceRole, and Source required, which " +
			"the SDK checks before it sends a call, so the fake doesn't. The API reference gives no default for " +
			"QueuedTimeoutInMinutes, and the user guide says 60, but AWS returned 480 in the recorded test; the fake fills " +
			"in 480. AWS reported the managed key as an alias ARN in the recorded test, which the fake does too, but the " +
			"fake keeps an alias the template sets, which AWS may also report as an ARN (the suite's OtherForm covers it). " +
			"The recorded test also returned Cache NO_CACHE and ImagePullCredentialsType CODEBUILD, which the docs give " +
			"no default for; the fake doesn't fill them in. The user guide says ConcurrentBuildLimit can't be more than " +
			"the account's limit, which the fake doesn't model. EnvironmentVariable's page says a variable's Type defaults to " +
			"PLAINTEXT, but the fake doesn't fill it in: the suite requires a read to return a template's lists as they " +
			"are, so the suite's OtherForm covers it instead.",
	},
	"CreateWebhook": {
		Doc: apiReference + "API_CreateWebhook.html",
		Rules: append(filterRules(func(groups [][]types.WebhookFilter) func(ctx context.Context, store *awsfake.CloudControl) error {
			return breakCreateWebhook(nil, func(in *codebuild.CreateWebhookInput) { in.FilterGroups = groups })
		}),
			webhookNameRule(func(ctx context.Context, store *awsfake.CloudControl, name string) error {
				_, err := newFakeAPI(store).CreateWebhook(ctx, &codebuild.CreateWebhookInput{ProjectName: aws.String(name)}, inRegion(region))
				return err
			}),
			nativetest.Rule{
				Rule: "the project exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).CreateWebhook(ctx, &codebuild.CreateWebhookInput{ProjectName: aws.String("app-build")}, inRegion(region))
					return err
				},
				Code: notFound,
			},
			nativetest.Rule{
				Rule: "the project has no webhook yet",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					create := breakCreateWebhook(nil, func(in *codebuild.CreateWebhookInput) {})
					if err := create(ctx, store); err != nil {
						return err
					}
					_, err := newFakeAPI(store).CreateWebhook(ctx, &codebuild.CreateWebhookInput{ProjectName: aws.String("app-build")}, inRegion(region))
					return err
				},
				Code: "ResourceAlreadyExistsException",
			},
			nativetest.Rule{
				Rule: "webhooks are for GitHub, GitHub Enterprise, GitLab, GitLab Self Managed, and Bitbucket sources",
				Doc:  userGuide + "webhooks.html",
				Break: breakCreateWebhook(func(in *codebuild.CreateProjectInput) {
					in.Source = &types.ProjectSource{Type: types.SourceTypeS3, Location: aws.String("bucket/app.zip")}
				}, func(in *codebuild.CreateWebhookInput) {}),
				Code: invalidInput,
			},
			nativetest.Rule{
				Rule:  "BuildType is BUILD, BUILD_BATCH, or RUNNER_BUILDKITE_BUILD",
				Break: breakCreateWebhook(nil, func(in *codebuild.CreateWebhookInput) { in.BuildType = "BUILD_ALL" }),
				Code:  invalidInput,
			},
			nativetest.Rule{
				Rule: "a scope configuration's scope is GITHUB_ORGANIZATION, GITHUB_GLOBAL, or GITLAB_GROUP",
				Doc:  scopeDoc,
				Break: breakCreateWebhook(nil, func(in *codebuild.CreateWebhookInput) {
					in.ScopeConfiguration = &types.ScopeConfiguration{Name: aws.String("example"), Scope: "GITHUB_REPOSITORY"}
				}),
				Code: invalidInput,
			},
			nativetest.Rule{
				Rule:  "BuildType RUNNER_BUILDKITE_BUILD is only for NO_SOURCE projects",
				Break: breakCreateWebhook(nil, func(in *codebuild.CreateWebhookInput) { in.BuildType = types.WebhookBuildTypeRunnerBuildkiteBuild }),
				Code:  invalidInput,
			},
			nativetest.Rule{
				Rule: "a scope configuration's scope is GITHUB_ORGANIZATION or GITHUB_GLOBAL for a GitHub source, and GITLAB_GROUP for a GitLab one",
				Doc:  scopeDoc,
				Break: breakCreateWebhook(nil, func(in *codebuild.CreateWebhookInput) {
					in.ScopeConfiguration = &types.ScopeConfiguration{Name: aws.String("example"), Scope: types.WebhookScopeTypeGitlabGroup}
				}),
				Code: invalidInput,
			},
			nativetest.Rule{
				Rule: "a scope configuration sets Domain when the source is GITHUB_ENTERPRISE or GITLAB_SELF_MANAGED",
				Doc:  scopeDoc,
				Break: breakCreateWebhook(func(in *codebuild.CreateProjectInput) { in.Source.Type = types.SourceTypeGithubEnterprise }, func(in *codebuild.CreateWebhookInput) {
					in.ScopeConfiguration = &types.ScopeConfiguration{Name: aws.String("example"), Scope: types.WebhookScopeTypeGithubOrganization}
				}),
				Code: invalidInput,
			},
		),
		Unclear: "The page's summary names only GitHub and Bitbucket sources, while the user guide adds GitHub Enterprise, " +
			"GitLab, and GitLab Self Managed, and BuildType allows a NO_SOURCE project with RUNNER_BUILDKITE_BUILD; neither " +
			"names the error for another source, nor for a scope or filter that breaks a rule. The fake returns " +
			"InvalidInputException, the page's error for input that isn't valid. The page says scope configurations are " +
			"only for GitHub, while ScopeConfiguration lists GITLAB_GROUP; the fake takes both. The user guide says an " +
			"organization or global webhook's project sets its source location to CODEBUILD_DEFAULT_WEBHOOK_SOURCE_LOCATION, " +
			"and that a global webhook is created manually and only takes WORKFLOW_JOB_QUEUED, without saying CreateWebhook " +
			"rejects otherwise; the fake doesn't check them. OAuthProviderException comes from the source provider's " +
			"credentials, which the fake doesn't model; tests inject it.",
	},
	"DeleteProject": {
		Doc: apiReference + "API_DeleteProject.html",
		Rules: []nativetest.Rule{
			{
				Rule: "Name has at least 1 character",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).DeleteProject(ctx, &codebuild.DeleteProjectInput{Name: aws.String("")}, inRegion(region))
					return err
				},
				Code: invalidInput,
			},
		},
		Unclear: "The page lists no ResourceNotFoundException and doesn't say what deleting a project that doesn't exist " +
			"does. The fake succeeds, and the handler also treats ResourceNotFoundException as gone. It doesn't say " +
			"whether the project's webhook goes with it; the handler deletes the webhook first.",
	},
	"DeleteWebhook": {
		Doc: apiReference + "API_DeleteWebhook.html",
		Rules: []nativetest.Rule{
			webhookNameRule(func(ctx context.Context, store *awsfake.CloudControl, name string) error {
				_, err := newFakeAPI(store).DeleteWebhook(ctx, &codebuild.DeleteWebhookInput{ProjectName: aws.String(name)}, inRegion(region))
				return err
			}),
			{
				Rule: "the project has a webhook",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := createProject(ctx, store, nil); err != nil {
						return err
					}
					_, err := newFakeAPI(store).DeleteWebhook(ctx, &codebuild.DeleteWebhookInput{ProjectName: aws.String("app-build")}, inRegion(region))
					return err
				},
				Code: notFound,
			},
		},
		Unclear: "The page doesn't say which error comes for a project without a webhook. The fake returns " +
			"ResourceNotFoundException, which the handler treats as already deleted. Its summary is for a project whose " +
			"source is in GitHub or Bitbucket, but it doesn't say the call fails for another source; an update that changes " +
			"the source and turns the webhook off calls UpdateProject first, and the fake doesn't check the source. " +
			"OAuthProviderException comes from the source provider's credentials, which the fake doesn't model.",
	},
	"ListProjects": {
		Doc: apiReference + "API_ListProjects.html",
		Rules: []nativetest.Rule{
			{
				Rule: "a page has at most 100 names, with a NextToken when there are more",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					f := newFakeAPI(store)
					for i := range 101 {
						if err := createProject(ctx, store, func(in *codebuild.CreateProjectInput) { in.Name = aws.String("project-" + strconv.Itoa(i)) }); err != nil {
							return err
						}
					}
					out, err := f.ListProjects(ctx, &codebuild.ListProjectsInput{}, inRegion(region))
					if err != nil {
						return err
					}
					if len(out.Projects) != 100 || out.NextToken == nil {
						return fmt.Errorf("the fake returned %d names in one page", len(out.Projects))
					}
					return nil
				},
			},
			{
				Rule: "NextToken has at least 1 character",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).ListProjects(ctx, &codebuild.ListProjectsInput{NextToken: aws.String("")}, inRegion(region))
					return err
				},
				Code: invalidInput,
			},
		},
	},
	"UpdateProject": {
		Doc: apiReference + "API_UpdateProject.html",
		Rules: []nativetest.Rule{
			{
				Rule: "Name has at least 1 character",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).UpdateProject(ctx, &codebuild.UpdateProjectInput{Name: aws.String("")}, inRegion(region))
					return err
				},
				Code: invalidInput,
			},
			{
				Rule: "the project exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).UpdateProject(ctx, &codebuild.UpdateProjectInput{Name: aws.String("app-build")}, inRegion(region))
					return err
				},
				Code: notFound,
			},
			{
				Rule:  "Description has at most 255 characters",
				Break: breakUpdate(func(in *codebuild.UpdateProjectInput) { in.Description = aws.String(strings.Repeat("a", 256)) }),
				Code:  invalidInput,
			},
			{
				Rule:  "TimeoutInMinutes is 5 to 2160",
				Break: breakUpdate(func(in *codebuild.UpdateProjectInput) { in.TimeoutInMinutes = aws.Int32(2161) }),
				Code:  invalidInput,
			},
			{
				Rule:  "QueuedTimeoutInMinutes is 5 to 480",
				Break: breakUpdate(func(in *codebuild.UpdateProjectInput) { in.QueuedTimeoutInMinutes = aws.Int32(4) }),
				Code:  invalidInput,
			},
			{
				Rule: "SecondarySources, SecondaryArtifacts, and SecondarySourceVersions have at most 12 items, and Tags at most 50",
				Break: breakUpdate(func(in *codebuild.UpdateProjectInput) {
					for i := range 51 {
						in.Tags = append(in.Tags, types.Tag{Key: aws.String("k" + strconv.Itoa(i)), Value: aws.String("v")})
					}
				}),
				Code: invalidInput,
			},
			{
				Rule: "ReportBuildStatus is only for GitHub, GitHub Enterprise, GitLab, GitLab Self Managed, and Bitbucket sources",
				Doc:  projectSourceDoc,
				Break: breakUpdate(func(in *codebuild.UpdateProjectInput) {
					in.Source = &types.ProjectSource{Type: types.SourceTypeNoSource, Buildspec: aws.String("version: 0.2"), ReportBuildStatus: aws.Bool(true)}
				}),
				Code: invalidInput,
			},
			{
				Rule: "the call changes only the settings it sends, and ConcurrentBuildLimit -1 removes the limit",
				Check: func(ctx context.Context, store *awsfake.CloudControl) error {
					err := createProject(ctx, store, func(in *codebuild.CreateProjectInput) {
						in.TimeoutInMinutes = aws.Int32(30)
						in.ConcurrentBuildLimit = aws.Int32(2)
					})
					if err != nil {
						return err
					}
					f := newFakeAPI(store)
					if _, err := f.UpdateProject(ctx, &codebuild.UpdateProjectInput{Name: aws.String("app-build"), Description: aws.String("Builds the app")}, inRegion(region)); err != nil {
						return err
					}
					if _, err := f.UpdateProject(ctx, &codebuild.UpdateProjectInput{Name: aws.String("app-build"), ConcurrentBuildLimit: aws.Int32(-1)}, inRegion(region)); err != nil {
						return err
					}
					p, err := readProject(ctx, store)
					if err != nil {
						return err
					}
					if aws.ToInt32(p.TimeoutInMinutes) != 30 || aws.ToString(p.Description) != "Builds the app" || p.Source == nil {
						return fmt.Errorf("the update changed settings it didn't send: %+v", p)
					}
					return nil
				},
			},
		},
		Unclear: limitsUnclear + " The page doesn't say what an empty value does, such as an empty Description, an empty " +
			"list of secondary sources, or an empty VpcConfig or BuildBatchConfig. The fake clears the setting, which the " +
			"handler relies on when a template removes it. It doesn't say what a read returns after ConcurrentBuildLimit " +
			"-1; the fake returns -1.",
	},
	"UpdateProjectVisibility": {
		Doc: apiReference + "API_UpdateProjectVisibility.html",
		Rules: []nativetest.Rule{
			{
				Rule: "ProjectVisibility is PUBLIC_READ or PRIVATE",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := createProject(ctx, store, nil); err != nil {
						return err
					}
					_, err := newFakeAPI(store).UpdateProjectVisibility(ctx, &codebuild.UpdateProjectVisibilityInput{ProjectArn: aws.String(arnPrefix + "app-build"), ProjectVisibility: "PUBLIC"}, inRegion(region))
					return err
				},
				Code: invalidInput,
			},
			{
				Rule: "ProjectArn has at least 1 character",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).UpdateProjectVisibility(ctx, &codebuild.UpdateProjectVisibilityInput{ProjectArn: aws.String(""), ProjectVisibility: types.ProjectVisibilityTypePrivate}, inRegion(region))
					return err
				},
				Code: invalidInput,
			},
			{
				Rule: "the project exists",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					_, err := newFakeAPI(store).UpdateProjectVisibility(ctx, &codebuild.UpdateProjectVisibilityInput{ProjectArn: aws.String(arnPrefix + "app-build"), ProjectVisibility: types.ProjectVisibilityTypePrivate}, inRegion(region))
					return err
				},
				Code: notFound,
			},
		},
		Unclear: "The page doesn't say what a call that leaves out ResourceAccessRole does to a role set before. The fake " +
			"removes it, which the handler relies on when a template removes ResourceAccessRole.",
	},
	"UpdateWebhook": {
		Doc: apiReference + "API_UpdateWebhook.html",
		Rules: append(filterRules(func(groups [][]types.WebhookFilter) func(ctx context.Context, store *awsfake.CloudControl) error {
			return breakUpdateWebhook(nil, "", func(in *codebuild.UpdateWebhookInput) { in.FilterGroups = groups })
		}),
			webhookNameRule(func(ctx context.Context, store *awsfake.CloudControl, name string) error {
				_, err := newFakeAPI(store).UpdateWebhook(ctx, &codebuild.UpdateWebhookInput{ProjectName: aws.String(name)}, inRegion(region))
				return err
			}),
			nativetest.Rule{
				Rule: "the project has a webhook",
				Break: func(ctx context.Context, store *awsfake.CloudControl) error {
					if err := createProject(ctx, store, nil); err != nil {
						return err
					}
					_, err := newFakeAPI(store).UpdateWebhook(ctx, &codebuild.UpdateWebhookInput{ProjectName: aws.String("app-build")}, inRegion(region))
					return err
				},
				Code: notFound,
			},
			nativetest.Rule{
				Rule:  "BuildType RUNNER_BUILDKITE_BUILD is only for NO_SOURCE projects",
				Break: breakUpdateWebhook(nil, "", func(in *codebuild.UpdateWebhookInput) { in.BuildType = types.WebhookBuildTypeRunnerBuildkiteBuild }),
				Code:  invalidInput,
			},
		),
		Unclear: "The page doesn't say BranchFilter can't be sent with FilterGroups, only that FilterGroups is " +
			"recommended; the handler never sends BranchFilter. It doesn't say what leaving out BuildType or " +
			"PullRequestBuildPolicy, or sending empty FilterGroups, does to the webhook's value. The fake clears BuildType " +
			"and FilterGroups, which the handler relies on, and keeps PullRequestBuildPolicy, so a template that removes " +
			"only PullRequestBuildPolicy may leave it in AWS. The call has no ScopeConfiguration, so the handler replaces " +
			"the webhook for a new scope. It doesn't name the error for a filter that breaks a rule; the fake returns " +
			"InvalidInputException. OAuthProviderException comes from the source provider's credentials, which the fake " +
			"doesn't model.",
	},
}
