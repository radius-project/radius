// @ts-nocheck

/** @param {{ github: any, core: any }} param0 */
export default async ({ github, core }) => {
  try {
    const remoteOwner = core.getInput('OWNER', { required: true });
    const remoteRepo = core.getInput('REPO', { required: true });
    const remoteWorkflowFile = core.getInput('WORKFLOW_FILE', { required: true });
    const dispatchStartedAt = core.getInput('DISPATCH_STARTED_AT', { required: true });

    const maxWaitSeconds = Number(core.getInput('MAX_WAIT_SECONDS') || '900'); // Default to 15 minutes
    const pollIntervalSeconds = Number(core.getInput('POLL_INTERVAL_SECONDS') || '10');

    /** @param {number} ms */
    const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

    core.info(`Waiting for remote workflow run in ${remoteOwner}/${remoteRepo}...`);

    const findLatestRun = async () => {
      const response = await github.rest.actions.listWorkflowRuns({
        owner: remoteOwner,
        repo: remoteRepo,
        workflow_id: remoteWorkflowFile,
        event: 'repository_dispatch',
        per_page: 20
      });

      const candidateRuns = (response.data.workflow_runs || [])
        /** @param {any} run */
        .filter((run) => run.created_at >= dispatchStartedAt)
        /** @param {any} left @param {any} right */
        .sort((left, right) => left.created_at.localeCompare(right.created_at));

      return candidateRuns.length > 0 ? candidateRuns[candidateRuns.length - 1] : undefined;
    };

    let run;
    for (let elapsed = 0; elapsed < maxWaitSeconds; elapsed += pollIntervalSeconds) {
      run = await findLatestRun();
      if (run) {
        break;
      }

      await sleep(pollIntervalSeconds * 1000);
    }

    if (!run) {
      core.setFailed(
        `Timed out waiting for remote workflow run to start: https://github.com/${remoteOwner}/${remoteRepo}/actions/workflows/${remoteWorkflowFile}`
      );
      return;
    }

    core.info(`Monitoring remote run id: ${run.id}`);

    for (let elapsed = 0; elapsed < maxWaitSeconds; elapsed += pollIntervalSeconds) {
      const runResponse = await github.rest.actions.getWorkflowRun({
        owner: remoteOwner,
        repo: remoteRepo,
        run_id: run.id
      });

      const status = runResponse.data.status;
      const conclusion = runResponse.data.conclusion || '';
      const htmlUrl = runResponse.data.html_url;

      if (status === 'completed') {
        core.setOutput('run_id', String(run.id));
        core.setOutput('run_url', htmlUrl);
        core.setOutput('conclusion', conclusion);

        core.info(`Remote workflow run URL: ${htmlUrl}`);

        if (conclusion === 'success') {
          core.info('Remote workflow completed successfully');
          return;
        }

        const jobsResponse = await github.rest.actions.listJobsForWorkflowRun({
          owner: remoteOwner,
          repo: remoteRepo,
          run_id: run.id,
          per_page: 100
        });

        const failedJobs = (jobsResponse.data.jobs || []).filter(
          /** @param {any} job */
          (job) => job.conclusion !== 'success'
        );
        const failedJobText = failedJobs
          /** @param {any} job */
          .map((job) => {
            const failedSteps = (job.steps || [])
              /** @param {any} step */
              .filter((step) => step.conclusion === 'failure')
              /** @param {any} step */
              .map((step) => `  - Step: ${step.name}`)
              .join('\n');

            return `- Job: ${job.name} [${job.conclusion}]${failedSteps ? `\n${failedSteps}` : ''}`;
          })
          .join('\n');

        core.setFailed(
          `Remote workflow failed with conclusion: ${conclusion}\nRemote workflow run: ${htmlUrl}${failedJobText ? `\nFailed jobs/steps:\n${failedJobText}` : ''}`
        );
        return;
      }

      await sleep(pollIntervalSeconds * 1000);
    }

    core.setOutput('run_id', String(run.id));
    core.setOutput('run_url', `https://github.com/${remoteOwner}/${remoteRepo}/actions/runs/${run.id}`);
    core.setFailed(`Timed out waiting for remote workflow completion: https://github.com/${remoteOwner}/${remoteRepo}/actions/runs/${run.id}`);
  } catch (error) {
    core.setFailed(error instanceof Error ? error.message : String(error));
  }
};
