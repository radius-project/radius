import { readFile } from "node:fs/promises";

export function releaseStatus({
  version,
  sourceSha,
  stage,
  state,
  releaseState,
  results = {},
  downstream = [],
  manifest,
  runUrl,
  durationSeconds = 0
}) {
  const recovery = `gh workflow run resume-release.yaml --repo radius-project/radius --ref main -f version=${version} -f source-commit=${sourceSha}`;
  const failures =
    manifest?.outputs?.filter((entry) => entry.status === "failed") ?? [];
  const stages = Object.entries(results).map(
    ([name, value]) => `${name}: ${value.result ?? value.outcome ?? value}`
  );
  const text = [
    `Release ${version}: ${stage} ${state}`,
    `Source: ${sourceSha}`,
    `GitHub Release: ${releaseState}`,
    `Elapsed: ${durationSeconds}s`,
    ...stages,
    ...Object.entries(manifest?.checks ?? {}).map(
      ([name, value]) => `${name}: ${value}`
    ),
    ...downstream.map(
      (entry) =>
        `${entry.name}: ${entry.state} ${entry.url ?? entry.recovery ?? ""}`
    )
  ];
  const markdown = [
    `## Release ${version}`,
    "",
    ...text.slice(1).map((line) => `- ${line}`),
    `- Stage: ${stage} (${state})`,
    `- Release identifier: ${version}-${sourceSha}`,
    `- Run: ${runUrl}`,
    "",
    "### Recovery",
    "",
    failures.length ?
      "Manifest conflicts require investigation; do not move tags or replace published artifacts."
    : "Incomplete stages can be retried with the same version and source. Completed work is reconciled.",
    "",
    "```sh",
    recovery,
    "```",
    "",
    "### Expected And Observed Outputs",
    "",
    manifest ?
      "```json\n" + JSON.stringify(manifest.outputs, null, 2) + "\n```"
    : "No complete manifest is available yet. Inspect the failed stage and retained verification evidence.",
    ""
  ].join("\n");
  const payload = {
    type: "message",
    attachments: [
      {
        contentType: "application/vnd.microsoft.card.adaptive",
        contentUrl: null,
        content: {
          $schema: "http://adaptivecards.io/schemas/adaptive-card.json",
          type: "AdaptiveCard",
          version: "1.4",
          body: [
            { type: "TextBlock", text: text[0], weight: "Bolder", wrap: true },
            {
              type: "TextBlock",
              text: text.slice(1).join("\n").slice(0, 6000),
              wrap: true
            },
            {
              type: "TextBlock",
              text:
                failures.length ?
                  `${failures.length} manifest conflicts; publication is blocked.`
                : recovery,
              wrap: true
            }
          ],
          actions: [
            { type: "Action.OpenUrl", title: "Release summary", url: runUrl }
          ]
        }
      }
    ]
  };
  return { markdown, payload };
}

export default async function reportReleaseStatus({ github, context, core }) {
  const version =
    core.getInput("VERSION") || context.ref?.replace("refs/tags/", "");
  const sourceSha = core.getInput("SOURCE_SHA") || context.sha;
  const stage = core.getInput("STAGE", { required: true });
  const results = JSON.parse(core.getInput("RESULTS") || "{}");
  const failed = Object.values(results).some((value) =>
    ["failure", "cancelled"].includes(value.result ?? value.outcome ?? value)
  );
  const state = failed ? "incomplete" : core.getInput("STATE") || "completed";
  let manifest;
  const file = core.getInput("MANIFEST_FILE");
  if (file) {
    try {
      manifest = JSON.parse(await readFile(file, "utf8"));
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
    }
  }
  const releases = await github.paginate(github.rest.repos.listReleases, {
    ...context.repo,
    per_page: 100
  });
  const release = releases.find((entry) => entry.tag_name === version);
  const { data: run } = await github.rest.actions.getWorkflowRun({
    ...context.repo,
    run_id: context.runId
  });
  const report = releaseStatus({
    version,
    sourceSha,
    stage,
    state,
    manifest,
    results,
    downstream: JSON.parse(core.getInput("DOWNSTREAM") || "[]"),
    releaseState:
      !release ? "absent"
      : release.draft ? "draft"
      : "published",
    runUrl: `${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`,
    durationSeconds: Math.max(
      0,
      Math.floor((Date.now() - Date.parse(run.run_started_at)) / 1000)
    )
  });
  await core.summary.addRaw(report.markdown).write();
  core.setOutput("teams-payload", JSON.stringify(report.payload));
}
