/*
Copyright 2023 The Radius Authors.

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

module.exports = async ({ github, context }) => {
    if (
        context.eventName == 'issue_comment' &&
        context.payload.action == 'created'
    ) {
        await handleIssueCommentCreate({ github, context })
    }
}

// Handle issue comment create event.
async function handleIssueCommentCreate({ github, context }) {
    const payload = context.payload
    const issue = context.issue
    const isFromPulls = !!payload.issue.pull_request
    const commentBody = payload.comment.body

    if (!commentBody) {
        console.log(
            '[handleIssueCommentCreate] comment body not found, exiting.'
        )
        return
    }
    const commandParts = commentBody.split(/\s+/)
    const command = commandParts.shift()


    switch (command) {
        case '/ok-to-test':
            await cmdOkToTest(github, issue, isFromPulls)
            break
        default:
            console.log(
                `[handleIssueCommentCreate] command ${command} not found, exiting.`
            )
            break
    }
}

/**
 * Trigger e2e test for the pull request.
 * @param {*} github GitHub object reference
 * @param {*} issue GitHub issue object
 * @param {boolean} isFromPulls is the workflow triggered by a pull request?
 */
async function cmdOkToTest(github, issue, isFromPulls) {
    if (!isFromPulls) {
        console.log(
            '[cmdOkToTest] only pull requests supported, skipping command execution.'
        )
        return
    }

    // Check if the user has permission to trigger e2e test with an issue comment
    checkTeamMembership(org, teamSlug, username, accessToken)
        .then(isMember => {
            if (!isMember) {
                console.log(`${username} is not a member of the ${teamSlug} team.`);
                return
            }
        })
        .catch(error => {
            console.error(error)
            return
        });


    // Get pull request
    const pull = await github.pulls.get({
        owner: issue.owner,
        repo: issue.repo,
        pull_number: issue.number,
    })

    if (pull && pull.data) {
        // Get commit id and repo from pull head
        const testPayload = {
            pull_head_ref: pull.data.head.sha,
            pull_head_repo: pull.data.head.repo.full_name,
            command: 'ok-to-test',
            issue: issue,
        }

        // Fire repository_dispatch event to trigger e2e test
        await github.repos.createDispatchEvent({
            owner: issue.owner,
            repo: issue.repo,
            event_type: 'e2e-test',
            client_payload: testPayload,
        })

        console.log(
            `[cmdOkToTest] triggered E2E test for ${JSON.stringify(
                testPayload
            )}`
        )
    }
}

async function checkTeamMembership(org, teamSlug, username, accessToken) {
    try {
      const response = await fetch(`https://api.github.com/orgs/${org}/teams/${teamSlug}/memberships/${username}`, {
        headers: {
          'Authorization': `Bearer ${accessToken}`,
          'Accept': 'application/vnd.github.v3+json'
        }
      });
  
      if (response.status === 200) {
        const data = await response.json();
        return data.state === 'active';
      } else if (response.status === 404) {
        return false;
      } else {
        throw new Error(`Error: ${response.statusText}`);
      }
    } catch (error) {
      throw error;
    }
  }