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

import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

export function readMainHistory() {
  return execFileSync(
    "git",
    ["rev-list", "--first-parent", "--reverse", "refs/remotes/origin/main"],
    { encoding: "utf8" }
  )
    .trim()
    .split(/\r?\n/);
}

export function orderByMainHistory(sources, history) {
  const positions = new Map(history.map((sha, index) => [sha, index]));
  const position = (pull) => {
    const sha = pull.merge_commit_sha ?? pull.mergeCommit?.oid;
    const index = positions.get(sha);
    if (index === undefined) {
      throw new Error(
        `Source PR #${pull.number} is not in the checked-out main merge history; refresh main and retry`
      );
    }
    return index;
  };
  const ranked = sources.map((pull) => ({ pull, index: position(pull) }));
  return ranked
    .sort((left, right) => left.index - right.index)
    .map(({ pull }) => pull);
}

function expectedBase(pull) {
  if (!pull.head.ref.startsWith("automation/prepare-release-")) {
    return "";
  }
  const body = pull.body ?? "";
  const start = "<!-- radius-release-plan:start -->";
  const end = "<!-- radius-release-plan:end -->";
  if (body.split(start).length !== 2 || body.split(end).length !== 2) {
    throw new Error("Generated release PR has no unique release plan");
  }
  const plan = body.split(start)[1].split(end)[0];
  const matches = [
    ...plan.matchAll(/^\s*productCommit:\s*([0-9a-f]{40})\s*$/gm)
  ];
  if (matches.length !== 1) {
    throw new Error("Release plan has no unique productCommit");
  }
  return matches[0][1];
}

export function backportEntry(pull, channel) {
  return {
    channel,
    source_pr: pull.number,
    source_commit: pull.merge_commit_sha,
    source_title: pull.title,
    source_url: pull.html_url,
    expected_base: expectedBase(pull)
  };
}

export function entriesForMergedPull(pull) {
  const channels = pull.labels
    .map((label) => label.name.match(/^backport release\/(\d+\.\d+)$/))
    .filter(Boolean)
    .map((match) => match[1]);
  return [...new Set(channels)]
    .sort()
    .map((channel) => backportEntry(pull, channel));
}

export function releaseChannelForEvent(context) {
  const branch =
    context.eventName === "push" ? context.ref.replace(/^refs\/heads\//, "")
    : context.payload.pull_request?.merged ?
      context.payload.pull_request.base.ref
    : "";
  return branch.match(/^release\/(\d+\.\d+)$/)?.[1] ?? "";
}

export async function findIncludedSourceCommits({
  github,
  owner,
  repo,
  channel,
  sources
}) {
  if (sources.length === 0) {
    return [];
  }
  const reference = `refs/heads/release/${channel}`;
  const { data: references } = await github.rest.git.listMatchingRefs({
    owner,
    repo,
    ref: `heads/release/${channel}`
  });
  const release = references.find((entry) => entry.ref === reference);
  if (!release) {
    return [];
  }

  const included = [];
  for (const source of sources) {
    const { data: comparison } =
      await github.rest.repos.compareCommitsWithBasehead({
        owner,
        repo,
        basehead: `${source.merge_commit_sha}...${release.object.sha}`,
        per_page: 1
      });
    if (comparison.status === "ahead" || comparison.status === "identical") {
      included.push(source.merge_commit_sha);
    } else if (
      comparison.status !== "behind" &&
      comparison.status !== "diverged"
    ) {
      throw new Error(`Unexpected ancestry result: ${comparison.status}`);
    }
  }
  return included;
}

export function selectNextBackport({
  channel,
  sources,
  mainHistory,
  openBackports,
  historicalBackports,
  includedSourceCommits = []
}) {
  if (
    openBackports.some((pull) =>
      pull.head.ref.startsWith("automation/backport-")
    )
  ) {
    return [];
  }

  const sourceByNumber = new Map(sources.map((pull) => [pull.number, pull]));
  const completed = new Set();
  for (const backport of historicalBackports.filter((pull) => pull.merged_at)) {
    const markers = [
      ...(backport.body ?? "").matchAll(
        /<!-- radius-backport-source: #(\d+) -->/g
      )
    ];
    if (markers.length !== 1) {
      continue;
    }
    const sourceNumber = Number(markers[0][1]);
    const source = sourceByNumber.get(sourceNumber);
    if (!source) {
      continue;
    }
    const trailer = `(cherry picked from commit ${source.merge_commit_sha})`;
    const hasTrailer = (backport.commits ?? []).some((entry) =>
      (entry.commit?.message ?? "").split(/\r?\n/).includes(trailer)
    );
    if (hasTrailer) {
      completed.add(sourceNumber);
    }
  }

  const included = new Set(includedSourceCommits);
  const pending = orderByMainHistory(sources, mainHistory).filter(
    (pull) =>
      !completed.has(pull.number) && !included.has(pull.merge_commit_sha)
  );
  return pending.length === 0 ? [] : [backportEntry(pending[0], channel)];
}

export async function selectReleaseBackports({
  github,
  context,
  channel = ""
}) {
  if (channel && !/^(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(channel)) {
    throw new Error("channel must use X.Y format");
  }
  channel ||= releaseChannelForEvent(context);
  const channels =
    channel ? [channel]
    : (
      context.eventName === "pull_request_target" &&
      context.payload.pull_request.merged &&
      context.payload.pull_request.base.ref === "main"
    ) ?
      entriesForMergedPull(context.payload.pull_request).map(
        (entry) => entry.channel
      )
    : [];
  if (channels.length === 0) return [];
  const mainHistory = readMainHistory();
  const selected = [];
  for (const channel of channels) {
    const repository = { owner: context.repo.owner, repo: context.repo.repo };
    const releaseBranch = `release/${channel}`;
    const openBackports = await github.paginate(github.rest.pulls.list, {
      ...repository,
      base: releaseBranch,
      state: "open",
      per_page: 100
    });
    const issues = await github.paginate(github.rest.issues.listForRepo, {
      ...repository,
      labels: `backport release/${channel}`,
      state: "closed",
      per_page: 100
    });
    const sources = [];
    for (const issue of issues.filter((item) => item.pull_request)) {
      const { data: pull } = await github.rest.pulls.get({
        ...repository,
        pull_number: issue.number
      });
      if (pull.merged_at && pull.base.ref === "main") sources.push(pull);
    }
    const historical = await github.paginate(github.rest.pulls.list, {
      ...repository,
      base: releaseBranch,
      state: "closed",
      per_page: 100
    });
    const historicalWithCommits = [];
    for (const pull of historical) {
      const commits =
        pull.merged_at ?
          await github.paginate(github.rest.pulls.listCommits, {
            ...repository,
            pull_number: pull.number,
            per_page: 100
          })
        : [];
      historicalWithCommits.push({ ...pull, commits });
    }
    const includedSourceCommits = await findIncludedSourceCommits({
      github,
      ...repository,
      channel,
      sources
    });
    selected.push(
      ...selectNextBackport({
        channel,
        sources,
        mainHistory,
        openBackports,
        historicalBackports: historicalWithCommits,
        includedSourceCommits
      })
    );
  }
  return selected;
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const [mode, file] = process.argv.slice(2);
  if (mode !== "--order-sources" || !file) {
    throw new Error(
      "usage: select-release-backports.mjs --order-sources <json>"
    );
  }
  const sources = JSON.parse(readFileSync(file, "utf8"));
  writeFileSync(
    file,
    JSON.stringify(orderByMainHistory(sources, readMainHistory()), null, 2) +
      "\n"
  );
}
