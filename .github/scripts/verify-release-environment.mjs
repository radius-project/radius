// ------------------------------------------------------------
// Copyright 2026 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

// @ts-check

/** @param {any} error */
function errorStatus(error) {
  return Number(error?.status ?? error?.response?.status ?? 0);
}

/**
 * Fail closed unless the approval environment can pause the controller.
 *
 * An environment without required reviewers lets its job start immediately,
 * so the controller would mutate release state without the documented
 * approval.
 *
 * @param {{github: any, context: any, core: any}} options
 */
export default async function verifyReleaseEnvironment({
  github,
  context,
  core
}) {
  const name = core.getInput("ENVIRONMENT", { required: true });
  let environment;
  try {
    ({ data: environment } = await github.rest.repos.getEnvironment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      environment_name: name
    }));
  } catch (error) {
    if (errorStatus(error) === 404) {
      throw new Error(
        `Environment ${name} does not exist, so release approval cannot pause the controller. Create it with required reviewers before releasing.`
      );
    }
    throw error;
  }

  const reviewerRules = (environment?.protection_rules ?? []).filter(
    (/** @type {any} */ rule) => rule?.type === "required_reviewers"
  );
  const hasReviewers = reviewerRules.some(
    (/** @type {any} */ rule) =>
      !Array.isArray(rule.reviewers) || rule.reviewers.length > 0
  );
  if (!hasReviewers) {
    throw new Error(
      `Environment ${name} has no required reviewers, so its approval job would pass without approval. Add required reviewers before releasing.`
    );
  }
  if (!environment.deployment_branch_policy) {
    core.warning(
      `Environment ${name} accepts deployments from any branch; restrict it to the default branch.`
    );
  }
  core.info(`Environment ${name} requires reviewer approval.`);
}
