// @ts-check

/** @param {import('@actions/github-script').AsyncFunctionArguments} AsyncFunctionArguments */
export default async ({ context, core }) => {
  try {
    if (!context?.eventName) {
      throw new Error("GitHub context is missing or invalid");
    }

    const eventName = context.eventName;
    const baseSha = core.getInput("BASE_SHA");
    const onlyChanged = core.getInput("ONLY_CHANGED");

    core.info(`Event name: ${eventName}`);
    core.info(`Base SHA: ${baseSha || "(not provided)"}`);

    let result;
    if (eventName === "pull_request" || eventName === "pull_request_target") {
      core.info(`PR event detected - using filter result: ${onlyChanged}`);
      result = onlyChanged;
    } else if (baseSha) {
      core.info(
        `Base SHA provided (PR-like context) - using filter result: ${onlyChanged}`,
      );
      result = onlyChanged;
    } else {
      core.info(
        "Non-PR event without base SHA - skipping filter (all jobs will run)",
      );
      result = "false";
    }

    core.setOutput("only_changed", result);
  } catch (error) {
    const message =
      error instanceof Error
        ? error.message
        : `Unexpected error: ${String(error)}`;
    core.setFailed(message);
  }
};
