// @ts-check

/** @param {import('@actions/github-script').AsyncFunctionArguments} AsyncFunctionArguments */
export default async ({ context, core }) => {
  try {
    if (!context?.eventName) {
      throw new Error("GitHub context is missing or invalid");
    }

    const eventName = context.eventName;
    const baseSha = core.getInput("BASE_SHA");

    core.info(`Event name: ${eventName}`);
    core.info(`Base SHA: ${baseSha || "(not provided)"}`);

    let result;
    let releaseMetadata = "false";
    if (
      eventName === "pull_request" ||
      eventName === "pull_request_target" ||
      eventName === "merge_group" ||
      baseSha
    ) {
      const allFiles = core.getInput("ALL_FILES_COUNT");
      const ignoredFiles = core.getInput("IGNORED_FILES_COUNT");
      const releaseFiles = core.getInput("RELEASE_FILES_COUNT");
      const workflowFiles = core.getInput("WORKFLOW_FILES_COUNT");
      const subsets = [ignoredFiles, releaseFiles, workflowFiles];
      if (
        [allFiles, ...subsets].some(
          (count) =>
            !/^(0|[1-9]\d*)$/.test(count) ||
            !Number.isSafeInteger(Number(count))
        ) ||
        subsets.some((count) => Number(count) > Number(allFiles))
      ) {
        throw new Error("Change filter returned invalid file counts");
      }
      releaseMetadata = String(Number(releaseFiles) > 0);
      result = String(
        Number(allFiles) > 0 &&
          allFiles === ignoredFiles &&
          !(releaseMetadata === "true" && Number(workflowFiles) > 0)
      );
      core.info(
        `PR-like event: ${ignoredFiles}/${allFiles} paths match the skip patterns; release=${releaseFiles}, workflows=${workflowFiles}`
      );
    } else {
      core.info(
        "Non-PR event without base SHA - skipping filter (all jobs will run)"
      );
      result = "false";
    }

    core.setOutput("only_changed", result);
    core.setOutput("release_metadata", releaseMetadata);
  } catch (error) {
    const message =
      error instanceof Error ?
        error.message
      : `Unexpected error: ${String(error)}`;
    core.setFailed(message);
  }
};
