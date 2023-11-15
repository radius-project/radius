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
    if (context.eventName === 'issue_comment' && context.payload.action === 'created') {
        try {
            await handleIssueCommentCreate({ github, context });
        } catch (error) {
            console.log(`[handleIssueCommentCreate] unexpected error: ${error}`);
        }
    }
}

// Handle issue comment create event.
async function handleIssueCommentCreate({ github, context }) {
    const payload = context.payload;
    const issue = context.issue;
    const isFromPulls = !!payload.issue.pull_request;
    const commentBody = payload.comment.body;
    const username = context.actor;

    if (!commentBody) {
        console.log('[handleIssueCommentCreate] comment body not found, exiting.');
        return;
    }

    const commandParts = commentBody.split(/\s+/);
    const command = commandParts.shift();

    switch (command) {
        case '/assign':
            await cmdAssign(github, issue, isFromPulls, username);
            break;
        default:
            console.log(`[handleIssueCommentCreate] command ${command} not found, exiting.`);
            break;
    }
}

/**
 * Assign issue to the user who commented.
 * @param {*} github GitHub object reference
 * @param {*} issue GitHub issue object
 * @param {*} isFromPulls is the workflow triggered by a pull request?
 * @param {*} username is the user who trigger the command
 */
async function cmdAssign(github, issue, isFromPulls, username) {
    if (isFromPulls) {
        console.log('[cmdAssign] pull requests not supported, skipping command execution.');
        return;
    } else if (issue.assignees && issue.assignees.length !== 0) {
        console.log('[cmdAssign] issue already has assignees, skipping command execution.');
        return;
    }

    await github.rest.issues.addAssignees({
        owner: issue.owner,
        repo: issue.repo,
        issue_number: issue.number,
        assignees: [username],
    });
}
