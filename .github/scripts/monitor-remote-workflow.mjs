// @ts-nocheck

const RELEASE_IDENTIFIER_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$/;
const EVENT_TYPE_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$/;
const API_RETRY_ATTEMPTS = 5;
const API_RETRY_BASE_DELAY_MS = 1000;
const API_RETRY_MAX_DELAY_MS = 15000;
const MAX_LOOKUP_PAGES = 5;
const RETRYABLE_NETWORK_CODES = new Set([
  "ECONNRESET",
  "ETIMEDOUT",
  "EAI_AGAIN",
  "UND_ERR_CONNECT_TIMEOUT",
]);

/** @param {string} name @param {string} value @param {number} fallback */
function positiveNumber(name, value, fallback) {
  const parsed = Number(value || String(fallback));
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(`${name} must be a positive number`);
  }
  return parsed;
}

/** @param {unknown} error */
function errorText(error) {
  return error instanceof Error ? error.message : String(error);
}

/** @param {any} error */
function isRetryableAPIError(error) {
  const status = Number(error?.status ?? error?.response?.status ?? 0);
  const headers = error?.response?.headers || {};
  return (
    status === 429 ||
    (status >= 500 && status <= 599) ||
    (status === 403 &&
      (headers["retry-after"] || headers["x-ratelimit-remaining"] === "0")) ||
    RETRYABLE_NETWORK_CODES.has(error?.code) ||
    RETRYABLE_NETWORK_CODES.has(error?.cause?.code)
  );
}

/** @param {any} error */
function retryAfterMilliseconds(error) {
  const value = Number(error?.response?.headers?.["retry-after"] ?? 0);
  return Number.isFinite(value) && value > 0 ? value * 1000 : 0;
}

/**
 * Dispatch or resume one correlated publisher run.
 *
 * @param {{
 *   github: any,
 *   core: any,
 *   sleep?: (ms: number) => Promise<void>,
 *   now?: () => number,
 *   random?: () => number,
 * }} options
 */
export default async ({
  github,
  core,
  sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
  now = () => Date.now(),
  random = () => Math.random(),
}) => {
  try {
    const remoteOwner = core.getInput("OWNER", { required: true });
    const remoteRepo = core.getInput("REPO", { required: true });
    const remoteWorkflowFile = core.getInput("WORKFLOW_FILE", {
      required: true,
    });
    const eventType = core.getInput("EVENT_TYPE", { required: true });
    const releaseIdentifier = core.getInput("RELEASE_IDENTIFIER", {
      required: true,
    });
    const clientPayloadText = core.getInput("CLIENT_PAYLOAD", {
      required: true,
    });

    if (!EVENT_TYPE_PATTERN.test(eventType)) {
      throw new Error(`Invalid event type: ${eventType}`);
    }
    if (!RELEASE_IDENTIFIER_PATTERN.test(releaseIdentifier)) {
      throw new Error(
        "Release identifier must contain 1-200 letters, numbers, dots, underscores, or hyphens",
      );
    }

    const clientPayload = JSON.parse(clientPayloadText);
    if (
      clientPayload === null ||
      Array.isArray(clientPayload) ||
      typeof clientPayload !== "object"
    ) {
      throw new Error("CLIENT_PAYLOAD must be a JSON object");
    }
    if (
      clientPayload.release_identifier &&
      clientPayload.release_identifier !== releaseIdentifier
    ) {
      throw new Error("CLIENT_PAYLOAD release_identifier does not match input");
    }
    clientPayload.release_identifier = releaseIdentifier;

    const maxWaitSeconds = positiveNumber(
      "MAX_WAIT_SECONDS",
      core.getInput("MAX_WAIT_SECONDS"),
      900,
    );
    const pollIntervalSeconds = positiveNumber(
      "POLL_INTERVAL_SECONDS",
      core.getInput("POLL_INTERVAL_SECONDS"),
      10,
    );
    const deadline = now() + maxWaitSeconds * 1000;
    const expectedRunName = `${eventType} / ${releaseIdentifier}`;

    const sleepWithinBudget = async (milliseconds) => {
      const remaining = deadline - now();
      if (remaining <= 0) {
        return false;
      }
      await sleep(Math.min(milliseconds, remaining));
      return now() < deadline;
    };

    const retryDelay = (attempt, error) => {
      const exponentialDelay = Math.min(
        API_RETRY_MAX_DELAY_MS,
        API_RETRY_BASE_DELAY_MS * 2 ** (attempt - 1),
      );
      const jitteredDelay =
        exponentialDelay / 2 + random() * (exponentialDelay / 2);
      return Math.max(Math.ceil(jitteredDelay), retryAfterMilliseconds(error));
    };

    const retryAPI = async (operationName, operation) => {
      for (let attempt = 1; attempt <= API_RETRY_ATTEMPTS; attempt += 1) {
        try {
          return await operation();
        } catch (error) {
          if (
            !isRetryableAPIError(error) ||
            attempt === API_RETRY_ATTEMPTS ||
            now() >= deadline
          ) {
            throw error;
          }

          const delay = retryDelay(attempt, error);
          core.info(
            `Transient GitHub API failure during ${operationName}; retrying in ${delay}ms (attempt ${attempt + 1}/${API_RETRY_ATTEMPTS})`,
          );
          if (!(await sleepWithinBudget(delay))) {
            throw error;
          }
        }
      }
      throw new Error(`${operationName} exhausted its retry budget`);
    };

    core.setOutput("release_identifier", releaseIdentifier);
    core.info(
      `Reconciling publisher run '${expectedRunName}' in ${remoteOwner}/${remoteRepo}`,
    );

    /** @param {any[]} runs */
    const correlatedRuns = (runs) =>
      (runs || [])
        .filter((run) => run.display_title === expectedRunName)
        .sort((left, right) => right.id - left.id);

    /** @param {any[]} runs */
    const selectReusableRun = (runs) =>
      runs.find(
        (run) => run.status === "completed" && run.conclusion === "success",
      ) || runs.find((run) => run.status !== "completed");

    const matchingRuns = async ({ allPages = true } = {}) => {
      const parameters = {
        owner: remoteOwner,
        repo: remoteRepo,
        workflow_id: remoteWorkflowFile,
        event: "repository_dispatch",
        per_page: 100,
      };

      if (!allPages) {
        const response = await retryAPI("recent publisher run lookup", () =>
          github.rest.actions.listWorkflowRuns(parameters),
        );
        return correlatedRuns(response.data.workflow_runs);
      }

      let pagesRead = 0;
      let matched = false;
      const runs = await retryAPI("publisher run lookup", () => {
        // Reset per attempt so a retried lookup gets the full page budget.
        pagesRead = 0;
        matched = false;
        return github.paginate(
          github.rest.actions.listWorkflowRuns,
          parameters,
          (response, done) => {
            const page = correlatedRuns(response.data.workflow_runs);
            pagesRead += 1;
            // Pages arrive newest first and one identifier's runs are
            // contiguous, so a long publisher history costs a bounded
            // number of API calls.
            if (
              pagesRead >= MAX_LOOKUP_PAGES ||
              (matched && page.length === 0)
            ) {
              done();
            }
            matched = matched || page.length > 0;
            return page;
          },
        );
      });
      return correlatedRuns(runs);
    };

    const dispatchRun = async (previousRunID) => {
      for (let attempt = 1; attempt <= API_RETRY_ATTEMPTS; attempt += 1) {
        try {
          await github.rest.repos.createDispatchEvent({
            owner: remoteOwner,
            repo: remoteRepo,
            event_type: eventType,
            client_payload: clientPayload,
          });
          return undefined;
        } catch (error) {
          if (!isRetryableAPIError(error)) {
            throw error;
          }

          const delay = Math.max(
            pollIntervalSeconds * 1000,
            retryDelay(attempt, error),
          );
          core.info(
            `Publisher dispatch returned a transient error; checking for '${expectedRunName}' before retrying`,
          );
          if (now() < deadline) {
            await sleepWithinBudget(delay);
          }

          const newRuns = (await matchingRuns({ allPages: false })).filter(
            (candidate) => candidate.id > previousRunID,
          );
          const discoveredRun = selectReusableRun(newRuns) || newRuns[0];
          if (discoveredRun) {
            core.info(
              `The uncertain dispatch created correlated run ${discoveredRun.id}; skipping redispatch`,
            );
            return discoveredRun;
          }
          if (attempt === API_RETRY_ATTEMPTS || now() >= deadline) {
            throw error;
          }
        }
      }
      throw new Error("Publisher dispatch exhausted its retry budget");
    };

    const existingRuns = await matchingRuns();
    const previousRunID = existingRuns.reduce(
      (highest, candidate) => Math.max(highest, candidate.id),
      0,
    );
    let run = selectReusableRun(existingRuns);
    if (!run) {
      core.info(
        existingRuns.length > 0
          ? `Previous correlated runs failed; dispatching retry '${eventType}'`
          : `No existing run found; dispatching '${eventType}'`,
      );
      run = await dispatchRun(previousRunID);

      while (!run && now() <= deadline) {
        const newRuns = (await matchingRuns({ allPages: false })).filter(
          (candidate) => candidate.id > previousRunID,
        );
        run = selectReusableRun(newRuns) || newRuns[0];
        if (run || now() >= deadline) {
          break;
        }
        await sleep(
          Math.min(pollIntervalSeconds * 1000, Math.max(0, deadline - now())),
        );
      }
    } else {
      core.info(`Reusing correlated remote run ${run.id}`);
    }

    if (!run) {
      core.setFailed(
        `Timed out waiting for publisher run '${expectedRunName}': https://github.com/${remoteOwner}/${remoteRepo}/actions/workflows/${remoteWorkflowFile}`,
      );
      return;
    }

    const runUrl =
      run.html_url ||
      `https://github.com/${remoteOwner}/${remoteRepo}/actions/runs/${run.id}`;
    core.setOutput("run_id", String(run.id));
    core.setOutput("run_url", runUrl);
    core.info(`Monitoring correlated remote run: ${runUrl}`);

    while (true) {
      if (run.status === "completed") {
        const conclusion = run.conclusion || "";
        core.setOutput("conclusion", conclusion);
        if (conclusion === "success") {
          core.info("Correlated remote workflow completed successfully");
          return;
        }

        const jobsResponse = await retryAPI("publisher job diagnostics", () =>
          github.rest.actions.listJobsForWorkflowRun({
            owner: remoteOwner,
            repo: remoteRepo,
            run_id: run.id,
            per_page: 100,
          }),
        );
        const failedJobs = (jobsResponse.data.jobs || []).filter(
          (job) => job.conclusion !== "success",
        );
        const failedJobText = failedJobs
          .map((job) => {
            const failedSteps = (job.steps || [])
              .filter((step) => step.conclusion === "failure")
              .map((step) => `  - Step: ${step.name}`)
              .join("\n");
            return `- Job: ${job.name} [${job.conclusion}]${failedSteps ? `\n${failedSteps}` : ""}`;
          })
          .join("\n");

        core.setFailed(
          `Correlated remote workflow failed with conclusion: ${conclusion}\nRemote workflow run: ${runUrl}${failedJobText ? `\nFailed jobs/steps:\n${failedJobText}` : ""}`,
        );
        return;
      }

      if (now() >= deadline) {
        core.setFailed(
          `Timed out waiting for correlated remote workflow completion: ${runUrl}`,
        );
        return;
      }
      await sleep(
        Math.min(pollIntervalSeconds * 1000, Math.max(0, deadline - now())),
      );
      const runResponse = await retryAPI("publisher run status", () =>
        github.rest.actions.getWorkflowRun({
          owner: remoteOwner,
          repo: remoteRepo,
          run_id: run.id,
        }),
      );
      run = runResponse.data;
    }
  } catch (error) {
    core.setFailed(errorText(error));
  }
};
