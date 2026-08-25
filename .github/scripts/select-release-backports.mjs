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

function expectedBase(pull) {
  if (!pull.head.ref.startsWith('automation/prepare-release-')) {
    return ''
  }
  const body = pull.body ?? ''
  const start = '<!-- radius-release-plan:start -->'
  const end = '<!-- radius-release-plan:end -->'
  if (body.split(start).length !== 2 || body.split(end).length !== 2) {
    throw new Error('Generated release PR has no unique release plan')
  }
  const plan = body.split(start)[1].split(end)[0]
  const matches = [
    ...plan.matchAll(/^\s*productCommit:\s*([0-9a-f]{40})\s*$/gm),
  ]
  if (matches.length !== 1) {
    throw new Error('Release plan has no unique productCommit')
  }
  return matches[0][1]
}

export function backportEntry(pull, channel) {
  return {
    channel,
    source_pr: pull.number,
    source_commit: pull.merge_commit_sha,
    source_title: pull.title,
    source_url: pull.html_url,
    expected_base: expectedBase(pull),
  }
}

export function entriesForMergedPull(pull) {
  const channels = pull.labels
    .map((label) => label.name.match(/^backport release\/(\d+\.\d+)$/))
    .filter(Boolean)
    .map((match) => match[1])
  return [...new Set(channels)]
    .sort()
    .map((channel) => backportEntry(pull, channel))
}

export function selectNextBackport({
  channel,
  sources,
  openBackports,
  historicalBackports,
}) {
  if (openBackports.some((pull) =>
    pull.head.ref.startsWith('automation/backport-')
  )) {
    return []
  }

  const sourceByNumber = new Map(sources.map((pull) => [pull.number, pull]))
  const completed = new Set()
  for (const backport of historicalBackports.filter((pull) => pull.merged_at)) {
    const markers = [
      ...(backport.body ?? '').matchAll(
        /<!-- radius-backport-source: #(\d+) -->/g,
      ),
    ]
    if (markers.length !== 1) {
      continue
    }
    const sourceNumber = Number(markers[0][1])
    const source = sourceByNumber.get(sourceNumber)
    if (!source) {
      continue
    }
    const trailer = `(cherry picked from commit ${source.merge_commit_sha})`
    const hasTrailer = (backport.commits ?? []).some((entry) =>
      (entry.commit?.message ?? '').split(/\r?\n/).includes(trailer)
    )
    if (hasTrailer) {
      completed.add(sourceNumber)
    }
  }

  const pending = sources
    .filter((pull) => !completed.has(pull.number))
    .sort((left, right) => left.number - right.number)
  return pending.length === 0 ? [] : [backportEntry(pending[0], channel)]
}