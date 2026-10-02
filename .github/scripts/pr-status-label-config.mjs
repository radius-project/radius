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

import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { load, YAMLException } from "js-yaml";

const CONFIG_PATH = new URL("../configs/pr-status-labels.yml", import.meta.url);
const REQUIRED_KEYS = Object.freeze({
  states: [
    "needsReviewer",
    "waitingForReview",
    "waitingForAuthor",
    "reviewApproved",
    "needsRebase",
    "readyForQueue"
  ],
  manual: ["needsAuthorResponse", "doNotMerge"]
});

function invalid(message) {
  throw new Error(`Invalid ${fileURLToPath(CONFIG_PATH)}: ${message}`);
}

function requireKeys(value, keys, section) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    invalid(`${section} must be a YAML mapping`);
  }
  const unknown = Object.keys(value).filter((key) => !keys.includes(key));
  if (unknown.length > 0) {
    invalid(`unknown keys in ${section}: ${unknown.join(", ")}`);
  }
  const missing = keys.filter((key) => !Object.hasOwn(value, key));
  if (missing.length > 0) {
    invalid(`missing keys in ${section}: ${missing.join(", ")}`);
  }
}

export default async () => {
  const source = await readFile(CONFIG_PATH, "utf8");
  let config;
  try {
    config = load(source, { filename: fileURLToPath(CONFIG_PATH) });
  } catch (error) {
    if (!(error instanceof YAMLException)) {
      throw error;
    }
    invalid(error.message);
  }
  requireKeys(config, Object.keys(REQUIRED_KEYS), "root");

  const names = new Map();
  for (const [section, keys] of Object.entries(REQUIRED_KEYS)) {
    requireKeys(config[section], keys, section);
    for (const key of keys) {
      const name = config[section][key];
      const field = `${section}.${key}`;
      if (
        typeof name !== "string" ||
        name.trim().length === 0 ||
        name !== name.trim() ||
        [...name].length > 50 ||
        /[\u0000-\u001f\u007f-\u009f]/u.test(name)
      ) {
        invalid(
          `${field} must be a nonempty label name of at most 50 characters, without surrounding whitespace or control characters`
        );
      }
      const normalized = name.toLowerCase();
      if (names.has(normalized)) {
        invalid(
          `${field} duplicates ${names.get(normalized)} (label names must be unique ignoring case)`
        );
      }
      names.set(normalized, field);
    }
    Object.freeze(config[section]);
  }
  return Object.freeze(config);
};
