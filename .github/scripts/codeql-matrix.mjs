// @ts-check

/**
 * Full matrix configuration for CodeQL analysis
 * @type {Array<{language: string, "build-mode": string, "working-directory": string}>}
 */
const FULL_MATRIX = [
  { language: "actions", "build-mode": "none", "working-directory": "." },
  { language: "go", "build-mode": "manual", "working-directory": "." },
  {
    language: "javascript",
    "build-mode": "none",
    "working-directory": "typespec",
  },
  { language: "custom-go", "build-mode": "none", "working-directory": "." },
];

/**
 * Mapping from matrix language to changed-files keys
 * @type {Record<string, string[]>}
 */
const LANGUAGE_TO_KEYS = {
  actions: ["actions"],
  go: ["go"],
  javascript: ["javascript"],
  "custom-go": ["go"], // GoSec analysis runs when Go files change
};

const FULL_MATRIX_TRIGGER = "/codeql full";

/**
 * Check if the PR body or comments contain the full matrix trigger
 * @param {import('@actions/github-script').AsyncFunctionArguments['context']} context
 * @param {import('@actions/github-script').AsyncFunctionArguments['github']} github
 * @param {import('@actions/github-script').AsyncFunctionArguments['core']} core
 * @returns {Promise<boolean>}
 */
async function shouldRunFullMatrix(context, github, core) {
  // Ensure payload exists
  if (!context.payload) {
    core.info("No payload available - skipping full matrix trigger check");
    return false;
  }

  // Check PR body
  const prBody = context.payload.pull_request?.body || "";
  if (prBody.includes(FULL_MATRIX_TRIGGER)) {
    core.info(`Found "${FULL_MATRIX_TRIGGER}" in PR body`);
    return true;
  }

  // Check PR comments
  const prNumber = context.payload.pull_request?.number;
  if (prNumber) {
    try {
      const { data: comments } = await github.rest.issues.listComments({
        owner: context.repo.owner,
        repo: context.repo.repo,
        issue_number: prNumber,
      });

      for (const comment of comments) {
        if (comment.body?.includes(FULL_MATRIX_TRIGGER)) {
          core.info(
            `Found "${FULL_MATRIX_TRIGGER}" in PR comment by ${comment.user?.login}`,
          );
          return true;
        }
      }
    } catch (error) {
      core.warning(`Failed to fetch PR comments: ${error}`);
    }
  }

  return false;
}

/** @param {import('@actions/github-script').AsyncFunctionArguments} AsyncFunctionArguments */
export default async ({ context, github, core }) => {
  try {
    if (!context?.eventName) {
      throw new Error("GitHub context is missing or invalid");
    }

    const modifiedKeysRaw = core.getInput("MODIFIED_KEYS") || "[]";
    const eventName = context.eventName;

    core.info(`Event name: ${eventName}`);
    core.info(`Modified keys (raw): ${modifiedKeysRaw}`);

    /** @type {string[]} */
    let modifiedKeys;
    try {
      modifiedKeys = JSON.parse(modifiedKeysRaw);
    } catch {
      core.warning(`Failed to parse MODIFIED_KEYS, using empty array`);
      modifiedKeys = [];
    }

    core.info(`Modified keys: ${JSON.stringify(modifiedKeys)}`);

    // For non-PR events (push, schedule), run all languages
    // For PR events, filter based on changed files unless "/codeql full" is found
    const isPrEvent =
      eventName === "pull_request" || eventName === "pull_request_target";

    let filteredMatrix;
    if (!isPrEvent) {
      core.info("Non-PR event detected - running full matrix");
      filteredMatrix = FULL_MATRIX;
    } else if (await shouldRunFullMatrix(context, github, core)) {
      core.info(`"${FULL_MATRIX_TRIGGER}" trigger found - running full matrix`);
      filteredMatrix = FULL_MATRIX;
    } else {
      core.info("PR event detected - filtering matrix based on changed files");
      filteredMatrix = FULL_MATRIX.filter((item) => {
        const requiredKeys = LANGUAGE_TO_KEYS[item.language] || [];
        return requiredKeys.some((key) => modifiedKeys.includes(key));
      });
    }

    core.info(`Filtered matrix: ${JSON.stringify(filteredMatrix)}`);

    // Output the matrix
    const matrixOutput =
      filteredMatrix.length > 0
        ? JSON.stringify({ include: filteredMatrix })
        : '{"include":[]}';

    core.info(`Matrix output: ${matrixOutput}`);
    core.setOutput("matrix", matrixOutput);
  } catch (error) {
    const message =
      error instanceof Error
        ? error.message
        : `Unexpected error: ${String(error)}`;
    core.setFailed(message);
  }
};
