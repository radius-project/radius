import { createHash } from "node:crypto";

const repository = "radius-project/radius";
const shaPattern = /^[0-9a-f]{40}$/;

function requireValue(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

export async function setup({ github, context, core }) {
  requireValue(
    context.payload.repository?.full_name === repository,
    "Cloud tests require the canonical repository"
  );
  const repo = { owner: "radius-project", repo: "radius" };
  const { data: main } = await github.rest.repos.getBranch({
    ...repo,
    branch: "main"
  });
  requireValue(
    main.protected && shaPattern.test(main.commit?.sha),
    "The uploader must come from protected main"
  );

  let sourceRepository = repository;
  let sourceSHA = main.commit.sha;
  let baseSHA = "";
  let prNumber = "";
  let deImage = "ghcr.io/radius-project/deployment-engine";
  let deTag = "latest";
  const event = context.eventName;
  if (event === "pull_request_target") {
    const pr = context.payload.pull_request;
    requireValue(
      pr?.base?.repo?.full_name === repository &&
        /^(main|features\/.+|release\/.+)$/.test(pr.base.ref) &&
        context.ref === `refs/heads/${pr.base.ref}`,
      "Unexpected PR base"
    );
    const { data: base } = await github.rest.repos.getBranch({
      ...repo,
      branch: pr.base.ref
    });
    requireValue(base.protected, "The PR controller branch must be protected");
    sourceRepository = pr.head?.repo?.full_name;
    sourceSHA = pr.head?.sha;
    baseSHA = pr.base.sha;
    requireValue(shaPattern.test(baseSHA), "Invalid PR base SHA");
    requireValue(
      Number.isSafeInteger(pr.number) && pr.number > 0,
      "Invalid PR number"
    );
    prNumber = String(pr.number);
  } else if (event === "merge_group") {
    const group = context.payload.merge_group;
    requireValue(
      group?.base_ref === "refs/heads/main" &&
        group.head_sha === context.sha &&
        context.ref.startsWith("refs/heads/gh-readonly-queue/main/"),
      "Unexpected merge-group source"
    );
    sourceSHA = context.sha;
  } else {
    requireValue(
      ["schedule", "repository_dispatch", "workflow_dispatch"].includes(
        event
      ) && context.ref === "refs/heads/main",
      "Dispatch the controller from protected main, not a candidate branch"
    );
    if (event === "workflow_dispatch") {
      const branch = context.payload.inputs?.branch;
      requireValue(
        typeof branch === "string" &&
          branch.length > 0 &&
          branch.length <= 255 &&
          !/[\u0000-\u0020\u007f]/u.test(branch),
        "A valid source branch is required"
      );
      const { data: source } = await github.rest.repos.getBranch({
        ...repo,
        branch
      });
      sourceSHA = source.commit?.sha;
    }
    if (event === "repository_dispatch") {
      requireValue(
        context.payload.action === "deployment-engine.run-functional-tests",
        "Unexpected repository dispatch"
      );
      deImage = context.payload.client_payload?.dest_image;
      deTag = context.payload.client_payload?.tag;
      requireValue(
        typeof deImage === "string" &&
          /^[a-z0-9.-]+\/[a-z0-9._/-]+$/.test(deImage) &&
          typeof deTag === "string" &&
          /^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$/.test(deTag),
        "Invalid deployment-engine reference"
      );
    }
  }
  requireValue(
    typeof sourceRepository === "string" &&
      /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(sourceRepository) &&
      shaPattern.test(sourceSHA),
    "The test source must identify an exact repository and commit"
  );
  let seed = `RADIUS|${context.sha}|${context.serverUrl}|${repository}|${context.runId}|${process.env.GITHUB_RUN_ATTEMPT}`;
  if (event === "schedule") {
    seed = `${context.runNumber}|${seed}`;
  }
  const uniqueID = `func${createHash("sha1").update(`${seed}\n`).digest("hex").slice(0, 10)}`;
  for (const [name, value] of Object.entries({
    REL_VERSION: `pr-${uniqueID}`,
    UNIQUE_ID: uniqueID,
    PR_NUMBER: prNumber,
    CHECKOUT_REPO: sourceRepository,
    CHECKOUT_REF: sourceSHA,
    TRUSTED_SHA: main.commit.sha,
    RAD_CLI_ARTIFACT_NAME: "rad_cli_linux_amd64",
    DE_IMAGE: deImage,
    DE_TAG: deTag,
    BASE_SHA: baseSHA
  })) {
    core.setOutput(name, value);
  }
}

export function testStatus(needs) {
  const required = [
    "authorize",
    "setup",
    "changes",
    "announce",
    "build",
    "upload-ghcr",
    "upload-test-types",
    "tests",
    "report-suite-results"
  ];
  const results = required.map((name) => needs[name]?.result);
  if (results.includes("failure")) {
    return "failure";
  }
  if (results.includes("cancelled")) {
    return "cancelled";
  }
  if (results.slice(0, 3).some((result) => result !== "success")) {
    return "failure";
  }
  const onlyChanged = needs.changes.outputs?.only_changed;
  if (onlyChanged === "true") {
    return results.slice(3).every((result) => result === "skipped") ?
        "success"
      : "failure";
  }
  return (
      onlyChanged === "false" && results.every((result) => result === "success")
    ) ?
      "success"
    : "failure";
}
