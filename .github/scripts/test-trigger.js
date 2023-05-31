
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