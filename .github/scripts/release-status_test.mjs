import assert from "node:assert/strict";
import test from "node:test";
import { releaseStatus } from "./release-status.mjs";

test("summaries expose release identity, expected state, conflicts, and exact recovery", () => {
  const sourceSha = "a".repeat(40);
  const report = releaseStatus({
    version: "v0.61.0",
    sourceSha,
    stage: "verification",
    state: "failure",
    releaseState: "draft",
    runUrl: "https://github.com/radius-project/radius/actions/runs/12",
    durationSeconds: 61,
    results: {
      build: { result: "success" },
      verification: { result: "failure" }
    },
    manifest: {
      outputs: [
        {
          name: "ucpd",
          status: "failed",
          expected: "sha256:expected",
          observed: "sha256:wrong"
        }
      ]
    }
  });
  for (const text of [
    sourceSha,
    "draft",
    "61s",
    "sha256:expected",
    "sha256:wrong",
    "do not move tags",
    "resume-release.yaml",
    "build: success"
  ]) {
    assert.ok(report.markdown.includes(text), text);
  }
  assert.equal(report.payload.attachments[0].content.type, "AdaptiveCard");
  assert.ok(JSON.stringify(report.payload).length < 28000);
});

test("summaries remain actionable when verification failed before producing a manifest", () => {
  const report = releaseStatus({
    version: "v0.61.0-rc.1",
    sourceSha: "b".repeat(40),
    stage: "staging",
    state: "failure",
    releaseState: "absent",
    runUrl: "https://example.test/run",
    downstream: [
      {
        name: "sample-tests",
        state: "incomplete",
        recovery: "Resume the same run"
      }
    ]
  });
  assert.match(report.markdown, /No complete manifest/);
  assert.match(report.markdown, /Resume the same run/);
});
