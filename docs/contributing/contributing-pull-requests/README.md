# Contributing pull requests

## Purpose

This guide explains how to open a pull request against [`radius-project/radius`](https://github.com/radius-project/radius): what to work on, how to prepare and submit your change, and what to expect during review. It is for anyone — first-time or returning contributors, human or agent — who wants a change merged into Radius. For a guided, end-to-end walkthrough of your first change, start with the [first commit guide](../contributing-code/contributing-code-first-commit/); this page is the authoritative reference for the pull-request process itself.

## Prerequisites

Before opening a pull request, make sure you have:

- **Agreement on scope.** For anything beyond a trivial fix (like a typo), [choose an existing issue](https://github.com/radius-project/radius/issues) or [open a new one](https://github.com/radius-project/radius/issues/new/choose) and work with the maintainers to confirm the change is in scope *before* writing code. The maintainers have discretion over what they accept — see [this article](https://www.igvita.com/2011/12/19/dont-push-your-pull-requests/) for why. If you have any doubt whether a contribution is valuable, ask first.
- **A fork of the repository.** Submit pull requests from a forked repo against the `main` branch (the default) unless otherwise instructed.
- **A working local build.** Run the basic validations (`make build test lint`) successfully before you submit. See [building the repo](../contributing-code/contributing-code-building/) for setup.

## Steps

### 1. Make your change and validate it locally

Work on your fork and run the basic validations before submitting:

```bash
make build test lint
```

This builds the repo, runs the unit tests, and runs the linters. If you get stuck, you can open the pull request anyway and ask for help in our [forum](https://discordapp.com/channels/1113519723347456110/1115302284356767814).

### 2. Write a good commit message

We value commit messages that are descriptive and meaningful at a glance. A good format to follow is:

```txt
<short description>

Fixes: #<issue>

<a longer description that includes>

- a summary of the changes being made
- the rationale for the change
- (optional) anything tricky or difficult as a heads up for reviewers
- (optional) additional follow up work that should be done (with links)
```

We **squash** pull requests as part of the merge process, so intermediate commit messages are appended. We prefer a single commit in the git history for each PR.

### 3. Sign your commits

The Developer Certificate of Origin (DCO) check requires every commit to be signed off. See [Signing your commits](../contributing-code/contributing-code-first-commit/first-commit-06-creating-a-pr/index.md#signing-your-commits) in the first commit guide for how to do this.

### 4. Open the pull request and fill out the template

Open the pull request from your fork against `main`. The form is pre-populated with our [template](https://github.com/radius-project/radius/blob/main/.github/pull_request_template.md). Fill it out to give your PR structure — a good commit message (step 2) makes this easy.

The template asks you to choose one of three change types. This helps us author the release notes and tells reviewers what to look for:

- **Bugfix** — the change fixes a case where Radius does not work as advertised, crashes, or fails internally.
- **Feature** — the change introduces new behavior or modifies an existing feature in a user-visible way.
- **Task** — a catch-all for changes with no direct user-visible impact (minor refactors, code/style cleanup, test improvements, comment fixes, build changes). Tasks are not included in the release notes.

### 5. (Optional) Self-review with the `radius-code-review` skill

If you use GitHub Copilot, you can run the [`radius-code-review`](../../../.github/skills/radius-code-review/SKILL.md) skill against your own pull request to generate an initial AI-assisted review *before* asking maintainers to look at it. This can help you catch obvious issues, missing tests, or unclear comments while you still own the change.

Prerequisites for the skill:

- Authenticated [`gh` CLI](https://cli.github.com/) and [`jq`](https://jqlang.org/) installed locally.
- One of: the [GitHub Copilot app](https://github.com/features/copilot), [GitHub Copilot CLI](https://docs.github.com/en/copilot/github-copilot-cli), or VS Code with the [GitHub Copilot Chat](https://marketplace.visualstudio.com/items?itemName=GitHub.copilot-chat) extension (with prompt files enabled — see VS Code's [prompt files docs](https://code.visualstudio.com/docs/copilot/copilot-customization#_prompt-files-experimental)).

Suggested workflow:

1. Push your branch and open the pull request.
2. Run the skill against your PR using one of:
   - **GitHub Copilot app**: open Copilot for this repository and ask `Use the radius-code-review skill to review PR #<your-pr-number>.` (or `/radius-code-review Review PR #<your-pr-number>`).
   - **Copilot CLI** (from the repo root): `/radius-code-review Review PR #<your-pr-number>`
   - **VS Code Copilot Chat**: type `/radius.code-review` in the chat input; VS Code will pick up `.github/prompts/radius.code-review.prompt.md` and prompt you for the PR number.
3. Read the generated `pr-analysis-<n>.md` and `pr-review-<n>.md` under `.copilot-tracking/`. Treat the output as a draft, not a verdict.
4. Apply the fixes you agree with, push the updates, and discard or push back on the suggestions you disagree with.
5. Do **not** post the AI-generated review to your own PR as-is. The script under `.copilot-tracking/pr-review-<n>.sh` is a starting point if you want to surface specific findings, but a human reviewer's review is still required for merge.

See the [code reviewing documentation](../contributing-code/contributing-code-reviewing/README.md#optional-ai-assisted-review-with-the-radius-code-review-skill) for the reviewer perspective on this skill.

### 6. Respond to review feedback

The maintainers or other contributors will add comments giving feedback, asking questions, and making suggestions. Respond to each comment to continue the discussion or explain whether you plan to address it. Accepting a pull request is ultimately at the maintainer's discretion.

- **Be proactive.** Comment on your own PR to point out relevant locations, decisions, opportunities for feedback, and tricky parts. This focuses reviewers' attention and saves them time.
- **Resolve feedback.** Mark comments as resolved once you've addressed them through discussion or a code change. If you are the reviewer, follow up (politely) if you feel your feedback hasn't been addressed adequately.
- **Anyone can participate.** We welcome any contributor or community member to engage with any pull request. Make suggestions and ask relevant questions; if a question is for your own learning, make it clear that it is "non-blocking." See the [code reviewing documentation](../contributing-code/contributing-code-reviewing/README.md) for full guidance.

## Verification

A pull request must pass these checkpoints to be accepted:

- **Initial review** — a maintainer reviews your summary and confirms an appropriate issue is linked.
- **Automated tests** — GitHub Actions workflows run unit, integration, and functional tests against your changes. Automation adds comments with links to logs so you can diagnose failures.
- **Contributor checklist** — the PR meets the [checklist requirements](https://github.com/radius-project/radius/blob/main/.github/pull_request_template.md#contributor-checklist).
- **Code review** — you receive and address feedback from a maintainer or other contributors.

The functional-tests workflow requires approval to run. One of our approvers is automatically notified when you submit the PR; once they approve the run, the functional tests start.

### How to get help with a pull request

- Notify the Radius Core team by commenting `@radius-project/radius-core-team` on your pull request.
- Post on Discord in the [#Forum channel](https://discord.gg/GJHN7kQrMh) to start a conversation.

## Troubleshooting

- **A functional-test run hasn't started.** The functional-tests workflow requires an approver to approve the run. Approvers are notified automatically when you submit; if a run hasn't started, wait for approval or ask the maintainers.
- **CodeQL reports a security issue.** We run [CodeQL](https://codeql.github.com/) for security analysis on every PR. It is not currently required to pass for a PR to be merged, as it may be triggered by other alerts in the repo. If CodeQL fails due to your changes, work with the maintainers to resolve it.
- **The spell check fails.** The PR check workflow runs [cspell](https://cspell.org/) with a [custom dictionary](https://github.com/radius-project/radius/blob/main/.cspellignore). Check the [workflow output](https://github.com/radius-project/radius/actions/workflows/spellcheck.yaml) for the flagged words and add correctly spelled words to `.cspellignore`. Run it locally from the repo root with:

  ```bash
  make spellcheck
  ```

  cspell requires [Node.js](https://nodejs.org/); install it globally with `npm install -g cspell`.
- **A CI failure you can't understand.** Our automation adds comments with links to logs. If you're still stuck, ask the maintainers for help.
- **Your PR was marked stale.** Pull requests inactive for 90 days are marked with a stale label and closed after a further 7 days of inactivity. This timeframe may be adjusted in the future based on project needs. Comment or push an update to keep your PR active.
