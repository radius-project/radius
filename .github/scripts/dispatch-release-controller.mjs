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

const versionPattern = /^v\d+\.\d+\.\d+(?:-rc\.[1-9]\d*)?$/;
const commitPattern = /^[0-9a-f]{40}$/;
const eventTypes = new Set([
  "release-controller.approve",
  "release-controller.resume"
]);

export async function resumeReleasePublication({
  github,
  context,
  core,
  now = Date.now,
  sleep = (milliseconds) =>
    new Promise((resolve) => setTimeout(resolve, milliseconds))
}) {
  const version = core.getInput("VERSION", { required: true });
  const sourceSha = core.getInput("SOURCE_SHA", { required: true });
  if (!versionPattern.test(version) || !commitPattern.test(sourceSha))
    throw new Error("Invalid publication resume identity");
  const deadline = now() + 300000;
  let run;
  while (!run) {
    const runs = await github.paginate(github.rest.actions.listWorkflowRuns, {
      ...context.repo,
      workflow_id: "build-release.yaml",
      head_sha: sourceSha,
      per_page: 100
    });
    const matching = runs.filter(
      (entry) =>
        entry.head_sha === sourceSha &&
        entry.head_branch === version &&
        ["push", "workflow_dispatch"].includes(entry.event)
    );
    run =
      matching
        .filter((entry) => entry.status !== "completed")
        .sort((left, right) => right.id - left.id)[0] ||
      matching.sort((left, right) => right.id - left.id)[0];
    if (!run) {
      if (now() >= deadline)
        throw new Error(
          `No tag-build run found for ${version} after five minutes; the tag exists. Start it with gh workflow run build-release.yaml --ref ${version}, then resume`
        );
      await sleep(10000);
    }
  }
  if (run.status === "completed" && run.conclusion !== "success") {
    const retry =
      run.conclusion === "failure" ?
        github.rest.actions.reRunWorkflowFailedJobs
      : github.rest.actions.reRunWorkflow;
    await retry({ ...context.repo, run_id: run.id, request: { retries: 0 } });
  }
  core.setOutput("run-url", run.html_url);
}

/** @param {{github: any, context: any, core: any}} options */
export default async function dispatchReleaseController({
  github,
  context,
  core
}) {
  const eventType = core.getInput("EVENT_TYPE", { required: true });
  const version = core.getInput("VERSION", { required: true });
  const sourceCommit = core.getInput("SOURCE_SHA", { required: true });
  if (!eventTypes.has(eventType)) {
    throw new Error("Unsupported release-controller event type");
  }
  if (!versionPattern.test(version)) {
    throw new Error("VERSION must use vX.Y.Z or vX.Y.Z-rc.N format");
  }
  if (!commitPattern.test(sourceCommit)) {
    throw new Error("SOURCE_SHA must be a full commit SHA");
  }

  await github.rest.repos.createDispatchEvent({
    owner: context.repo.owner,
    repo: context.repo.repo,
    event_type: eventType,
    client_payload: {
      version,
      source_commit: sourceCommit,
      requested_by: context.actor
    }
  });
  core.setOutput("release-identifier", `${version}-${sourceCommit}`);
}
