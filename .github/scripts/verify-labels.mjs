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

import { readFileSync } from "node:fs";
import { join } from "node:path";

export function readCatalog() {
  const path = join(
    process.env.GITHUB_WORKSPACE || process.cwd(),
    ".github",
    "labels.yml"
  );
  const lines = readFileSync(path, "utf8")
    .split(/\r?\n/)
    .filter((line) => line.trim() && !line.startsWith("#") && line !== "---");
  if (lines.length === 0 || lines.length % 3 !== 0) {
    throw new Error(
      "Label catalog must contain name, color and description entries"
    );
  }

  const labels = [];
  const names = new Set();
  for (let index = 0; index < lines.length; index += 3) {
    const name = /^- name: ([\w:/.-]+)$/.exec(lines[index])?.[1];
    const color = /^  color: ([0-9a-fA-F]{6})$/.exec(lines[index + 1])?.[1];
    const description = /^  description: (.+)$/.exec(lines[index + 2])?.[1];
    if (!name || !color || !description || description.length > 100) {
      throw new Error(`Unsupported label catalog entry near line ${index + 1}`);
    }
    if (names.has(name.toLowerCase())) {
      throw new Error(`Duplicate label in catalog: ${name}`);
    }
    names.add(name.toLowerCase());
    labels.push({ name, color, description });
  }
  return labels;
}

/** @param {import('@actions/github-script').AsyncFunctionArguments} AsyncFunctionArguments */
export default async ({ github, context, core }) => {
  const expected = readCatalog();
  const actual = await github.paginate(github.rest.issues.listLabelsForRepo, {
    ...context.repo,
    per_page: 100
  });
  const byName = new Map(
    actual.map((label) => [label.name.toLowerCase(), label])
  );
  const mismatches = [];
  for (const label of expected) {
    const current = byName.get(label.name.toLowerCase());
    if (!current) {
      mismatches.push(`${label.name}: missing`);
    } else if (
      current.name !== label.name ||
      current.color.toLowerCase() !== label.color.toLowerCase() ||
      (current.description ?? "") !== label.description
    ) {
      mismatches.push(`${label.name}: name, color or description differs`);
    }
  }
  if (mismatches.length > 0) {
    for (const mismatch of mismatches) {
      core.error(mismatch);
    }
    throw new Error(`Failed to sync ${mismatches.length} catalog label(s)`);
  }
  core.info(`Verified all ${expected.length} repository labels in the catalog`);
};
