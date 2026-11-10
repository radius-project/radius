/*
Copyright 2026 The Radius Authors.

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

package kubernetes

import (
	"context"
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestProviderTransitionOutageConfirmation(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"default no", "yes prompt", "yes flag", "missing prompt", "preflight only",
		"failed preflight", "failed upgrade", "skip preflight still checks", "prompt failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			controller := gomock.NewController(t)
			client := helm.NewMockInterface(controller)
			log := output.NewMockInterface(controller)
			log.EXPECT().LogInfo(gomock.Any()).AnyTimes()
			questions := prompt.NewMockInterface(controller)
			runner := &Runner{Helm: client, Output: log, Prompter: questions,
				MTLS: helm.MTLSProviderOptions{Provider: "external"}, KubeContext: "selected-context",
				Set: []string{"de.externalIdentity.enabled=true"}}
			client.EXPECT().CheckRadiusInstall("selected-context").Return(helm.InstallState{
				RadiusInstalled: true, RadiusVersion: "0.48.0"}, nil)
			if scenario == "yes flag" || scenario == "skip preflight still checks" {
				runner.AssumeYes = true
			}
			runner.SkipPreflight = scenario == "skip preflight still checks"
			runner.PreflightOnly = scenario == "preflight only"
			if scenario == "missing prompt" {
				runner.Prompter = nil
			}
			preflightFailure := errors.New("live issuer policy refused the target")
			client.EXPECT().CheckProviderTransition(gomock.Any(), gomock.Any(), "selected-context").
				DoAndReturn(func(_ context.Context, options helm.ClusterOptions, _ string) (bool, error) {
					require.False(t, options.MTLS.OutageConfirmed)
					require.Empty(t, options.Radius.ChartVersion, "provider changes must not implicitly select the CLI's chart version")
					require.Equal(t, runner.Set, options.Radius.SetArgs)
					if scenario == "failed preflight" {
						return true, preflightFailure
					}
					return true, nil
				})
			if scenario == "default no" || scenario == "yes prompt" || scenario == "failed upgrade" || scenario == "prompt failure" {
				answer := prompt.ConfirmYes
				if scenario == "default no" {
					answer = prompt.ConfirmNo
				}
				var err error
				if scenario == "prompt failure" {
					err = errors.New("input unavailable")
				}
				questions.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, gomock.Any()).Return(answer, err)
			}
			upgraded := scenario == "yes prompt" || scenario == "yes flag" ||
				scenario == "failed upgrade" || scenario == "skip preflight still checks"
			if upgraded {
				client.EXPECT().UpgradeRadius(gomock.Any(), gomock.Any(), "selected-context").
					DoAndReturn(func(_ context.Context, options helm.ClusterOptions, _ string) error {
						require.True(t, options.MTLS.OutageConfirmed)
						if scenario == "failed upgrade" {
							return errors.New("transition retained at verifying")
						}
						return nil
					})
			}
			err := runner.Run(t.Context())
			switch scenario {
			case "missing prompt":
				require.ErrorContains(t, err, "--yes")
			case "failed preflight":
				require.ErrorIs(t, err, preflightFailure)
			case "failed upgrade":
				require.ErrorContains(t, err, "retained")
			case "prompt failure":
				require.ErrorContains(t, err, "input unavailable")
			default:
				require.NoError(t, err)
			}
		})
	}
}
